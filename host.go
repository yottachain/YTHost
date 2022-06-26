package host

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/rpc"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	mnet "github.com/multiformats/go-multiaddr-net"
	"github.com/sirupsen/logrus"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/config"
	"github.com/yottachain/YTHost/connAutoCloser"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/peerInfo"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

type host struct {
	cfg      *config.Config
	listener mnet.Listener
	srv      *rpc.Server
	service.HandlerMap
	clientStore *clientStore.ClientStore
	httpClient  *http.Client
	Cs          *stat.ConnStat
}

func NewHost(options ...option.Option) (*host, error) {
	hst := new(host)
	hst.cfg = config.NewConfig()
	for _, bindOp := range options {
		bindOp(hst.cfg)
	}
	ls, err := mnet.Listen(hst.cfg.ListenAddr)
	if err != nil {
		return nil, err
	}
	hst.listener = ls
	srv := rpc.NewServer()
	hst.srv = srv
	hst.HandlerMap = make(service.HandlerMap)
	hst.Cs = stat.NewCs()
	hst.clientStore = clientStore.NewClientStore(hst.Connect)
	hst.httpClient = &http.Client{}
	return hst, nil
}

func (hst *host) Accept() {
	addrService := new(service.AddrService)
	addrService.Info.ID = hst.cfg.ID
	addrService.Info.Addrs = hst.Addrs()
	addrService.PubKey = hst.Config().Privkey.GetPublic()
	addrService.Version = hst.Config().Version

	msgService := new(service.MsgService)
	msgService.Handler = hst.HandlerMap
	msgService.Pi = peerInfo.PeerInfo{ID: hst.cfg.ID, Addrs: hst.Addrs()}

	if err := hst.srv.RegisterName("as", addrService); err != nil {
		logrus.Panicf("[Host]%s\n", err)
	}
	if err := hst.srv.RegisterName("ms", msgService); err != nil {
		logrus.Panicf("[Host]%s\n", err)
	}
	lis := mnet.NetListener(hst.listener)
	for {
		conn, err := lis.Accept()
		if err != nil {
			logrus.Errorf("[Host]rpc.Serve: accept:%s\n", err.Error())
			continue
		}
		hst.Cs.SccAdd()
		ac := connAutoCloser.New(conn)
		ac.SetOuttime(time.Minute * 5)
		go func() {
			hst.srv.ServeConn(ac)
			hst.Cs.SccSub()
			ac.Stop()
		}()
	}
}

func (h *host) SendHTTPMsg(ma multiaddr.Multiaddr, mid int32, msg []byte) ([]byte, error) {
	addr, err := ma.ValueForProtocol(multiaddr.P_DNS4)
	if err != nil {
		ip, err := ma.ValueForProtocol(multiaddr.P_IP4)
		if err != nil {
			return nil, err
		}
		addr = ip
	}
	port, err := ma.ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer([]byte{})
	err = binary.Write(buf, binary.BigEndian, msg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%s/msg/%d", addr, port, mid), buf)
	if err != nil {
		return nil, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 500 && resp.StatusCode >= 400 {
		return nil, fmt.Errorf("requeset 40X error")
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("response 50X error")
	}
	respData, err := ioutil.ReadAll(resp.Body)
	return respData, err
}

func (hst *host) Listenner() mnet.Listener {
	return hst.listener
}

func (hst *host) Server() *rpc.Server {
	return hst.srv
}

func (hst *host) Config() *config.Config {
	return hst.cfg
}

func (hst *host) ConnStat() *stat.ConnStat {
	return hst.Cs
}

func (hst *host) ClientStore() *clientStore.ClientStore {
	return hst.clientStore
}

func (hst *host) Addrs() []multiaddr.Multiaddr {
	port, err := hst.listener.Multiaddr().ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return nil
	}
	tcpMa, err := multiaddr.NewMultiaddr(fmt.Sprintf("/tcp/%s", port))
	if err != nil {
		return nil
	}
	var res []multiaddr.Multiaddr
	maddrs, err := mnet.InterfaceMultiaddrs()
	if err != nil {
		return nil
	}
	for _, ma := range maddrs {
		newMa := ma.Encapsulate(tcpMa)
		if mnet.IsIPLoopback(newMa) {
			continue
		}
		res = append(res, newMa)
	}
	return res
}

func (hst *host) Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	conn, err := hst.connect(ctx, pid, mas)
	if err != nil {
		return nil, err
	}
	clt := rpc.NewClient(conn)
	ytclt, err := client.WarpClient(clt,
		&peer.AddrInfo{ID: hst.cfg.ID, Addrs: hst.Addrs()},
		hst.cfg.Privkey.GetPublic(),
		hst.Config().Version,
		hst.Cs,
	)
	if err != nil {
		_ = clt.Close()
		return nil, err
	}
	ytclt.Cs.CccAdd()
	return ytclt, nil
}

func (hst *host) connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (mnet.Conn, error) {
	size := len(mas)
	resChan := make(chan interface{}, len(mas))
	var isOK int32 = 0
	for _, addr := range mas {
		go func(addr multiaddr.Multiaddr) {
			d := &mnet.Dialer{}
			if conn, err := d.DialContext(ctx, addr); err == nil {
				if atomic.AddInt32(&isOK, 1) > 1 {
					conn.Close()
				} else {
					resChan <- conn
				}
			} else {
				if atomic.LoadInt32(&isOK) == 0 {
					resChan <- err
				}
			}
		}(addr)
	}
	var errRes error
	for ii := 0; ii < size; ii++ {
		select {
		case <-ctx.Done():
			atomic.AddInt32(&isOK, 1)
			return nil, fmt.Errorf("conn ctx quit")
		case res := <-resChan:
			if conn, ok := res.(mnet.Conn); ok {
				return conn, nil
			} else {
				errRes = res.(error)
			}
		}
	}
	return nil, fmt.Errorf("dail all maddr fail %s", errRes)
}

func (hst *host) ConnectAddrStrings(ctx context.Context, id string, addrs []string) (*client.YTHostClient, error) {
	buf, _ := base58.Decode(id)
	pid, err := peer.IDFromBytes(buf)
	if err != nil {
		return nil, err
	}
	var mas = make([]multiaddr.Multiaddr, len(addrs))
	for k, v := range addrs {
		ma, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			continue
		}
		mas[k] = ma
	}
	return hst.Connect(ctx, pid, mas)
}

func (hst *host) SendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) ([]byte, error) {
	clt, ok := hst.ClientStore().GetClient(pid)
	if !ok {
		return nil, fmt.Errorf("no client ID is:%s", pid.Pretty())
	}
	res, err := clt.SendMsg(ctx, mid, msg)
	return res, err
}

func (hst *host) AsyncSendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) (*rpc.Call, error) {
	clt, ok := hst.ClientStore().GetClient(pid)
	if !ok {
		return nil, fmt.Errorf("no client ID is:%s", pid.Pretty())
	}
	return clt.AsyncSendMsg(ctx, mid, msg)
}

func (hst *host) SendMsgAuto(ctx context.Context, pid peer.ID, mid int32, ma multiaddr.Multiaddr, msg []byte) ([]byte, error) {
	if _, err := ma.ValueForProtocol(multiaddr.P_HTTP); err == nil {
		return hst.SendHTTPMsg(ma, mid, msg)
	} else {
		clt, err := hst.clientStore.Get(ctx, pid, []multiaddr.Multiaddr{ma})
		if err != nil {
			return nil, err
		}
		return clt.SendMsg(ctx, mid, msg)
	}
}
