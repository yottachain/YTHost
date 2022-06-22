package host

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/rpc"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	mnet "github.com/multiformats/go-multiaddr-net"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/config"
	"github.com/yottachain/YTHost/connAutoCloser"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/peerInfo"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

//type Host interface {
//	Accept()
//	Addrs() []multiaddr.Multiaddr
//	Server() *rpc.Server
//	Config() *config.Config
//	Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
//	RegisterHandler(id service.MsgId, handlerFunc service.Handler)
//}

type host struct {
	cfg      *config.Config
	listener mnet.Listener
	listenerCmd mnet.Listener
	srv      *rpc.Server
	service.HandlerMap
	clientStore *clientStore.ClientStore
	httpClient  *http.Client
	Cs *stat.ConnStat
}

func NewHost(options ...option.Option) (*host, error) {
	hst := new(host)
	hst.cfg = config.NewConfig()

	for _, bindOp := range options {
		bindOp(hst.cfg)
	}

	if hst.cfg.ListenAddr != nil {
		ls, err := mnet.Listen(hst.cfg.ListenAddr)
		if err != nil {
			return nil, err
		}else {
			hst.listener = ls
		}
	} else {
		return nil, fmt.Errorf("listen addr is nil")
	}


	if hst.cfg.ListenAddrCmd != nil &&
		!(hst.cfg.ListenAddrCmd.Equal(hst.cfg.ListenAddr)){
		ls, err := mnet.Listen(hst.cfg.ListenAddrCmd)
		if err == nil {
			hst.listenerCmd = ls
		} else {
			hst.listenerCmd = nil
		}
	} else {
		hst.listenerCmd = nil
	}


	srv := rpc.NewServer()
	hst.srv = srv

	hst.HandlerMap = make(service.HandlerMap)

	hst.Cs = stat.NewCs()
	hst.clientStore = clientStore.NewClientStore(hst.Connect)

	if hst.cfg.PProf != "" {
		go func() {
			if err := http.ListenAndServe(hst.cfg.PProf, nil); err != nil {
				fmt.Println("PProf open fail:", err)
			} else {
				fmt.Println("PProf debug open:", hst.cfg.PProf)
			}
		}()
	}

	hst.httpClient = &http.Client{}

	return hst, nil
}

func (hst *host) Accept() {
	ctx := context.Background()

	addrService := new(service.AddrService)
	addrService.Info.ID = hst.cfg.ID
	addrService.Info.Addrs, _ = hst.Addrs(hst.listener)
	addrService.PubKey = hst.Config().Privkey.GetPublic()
	addrService.Version = hst.Config().Version

	msgService := new(service.MsgService)
	msgService.Handler = hst.HandlerMap

	adds, _ := hst.Addrs(hst.listener)
	msgService.Pi = peerInfo.PeerInfo{ID:hst.cfg.ID, Addrs:adds}

	if err := hst.srv.RegisterName("as", addrService); err != nil {
		panic(err)
	}

	if err := hst.srv.RegisterName("ms", msgService); err != nil {
		panic(err)
	}

	go func() {
		if hst.listener != nil {
			lis := mnet.NetListener(hst.listener)
			for {
				conn, err := lis.Accept()
				if err != nil {
					fmt.Println("rpc.Serve: accept:", err.Error())
					continue
				}
				go hst.Cs.SccAdd()
				ac := connAutoCloser.New(conn)
				ac.SetOuttime(time.Minute * 5)
				go func() {
					hst.srv.ServeConn(ac)
					hst.Cs.SerCloseChanl <- struct{}{}
				}()
			}
		}
	}()

	go func() {
		if hst.listenerCmd != nil {
			lis := mnet.NetListener(hst.listenerCmd)
			for {
				conn, err := lis.Accept()
				if err != nil {
					fmt.Println("rpc.Serve: accept:", err.Error())
					continue
				}
				go hst.Cs.SccAdd()
				ac := connAutoCloser.New(conn)
				ac.SetOuttime(time.Minute * 5)
				go func() {
					hst.srv.ServeConn(ac)
					hst.Cs.SerCloseChanl <- struct{}{}
				}()
			}
		}
	}()

	<-ctx.Done()
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

func (hst *host) Listenner() (mnet.Listener, mnet.Listener ){
	return hst.listener, hst.listenerCmd
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

func (hst *host) Addrs(ls mnet.Listener) ([]multiaddr.Multiaddr, string) {
	if ls == nil {
		return nil, ""
	}

	port, err := ls.Multiaddr().ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return nil, ""
	}

	tcpMa, err := multiaddr.NewMultiaddr(fmt.Sprintf("/tcp/%s", port))

	if err != nil {
		return nil, ""
	}

	var res []multiaddr.Multiaddr
	maddrs, err := mnet.InterfaceMultiaddrs()
	if err != nil {
		return nil, ""
	}

	for _, ma := range maddrs {
		newMa := ma.Encapsulate(tcpMa)
		if mnet.IsIPLoopback(newMa) {
			continue
		}
		res = append(res, newMa)
	}
	return res, port
}

// Connect 连接远程节点
func (hst *host) Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	conn, err := hst.connect(ctx, pid, mas)
	if err != nil {
		return nil, err
	}

	clt := rpc.NewClient(conn)
	Adds, _ := hst.Addrs(hst.listener)
	ytclt, err := client.WarpClient(clt,
		&peer.AddrInfo{
		ID:hst.cfg.ID,
		Addrs: Adds,
		},
		hst.cfg.Privkey.GetPublic(),
		hst.Config().Version,
		hst.Cs,
	)
	if err != nil {
		_ = clt.Close()
		return nil, err
	}

	go ytclt.Cs.CccAdd()

	return ytclt, nil
}

func (hst *host)SendMsgAuto(ctx context.Context, pid peer.ID,mid int32,  ma multiaddr.Multiaddr,msg []byte) ([]byte,error) {
	log.Printf("[YTHost]mid %x, buf len %d\n",mid,len(msg))
	if _, err := ma.ValueForProtocol(multiaddr.P_HTTP);err ==nil {
		return hst.SendHTTPMsg(ma,mid,msg)
	} else {
		clt, err := hst.clientStore.Get(ctx,pid,[]multiaddr.Multiaddr{ma})
		if err != nil {
			return nil,err
		}
		return clt.SendMsg(ctx,mid,msg)
	}
}

func (hst *host) connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (mnet.Conn, error) {
	connChan := make(chan mnet.Conn)
	errChan := make(chan error)
	wg := sync.WaitGroup{}
	wg.Add(len(mas))
	var errRes string

	go func() {
		wg.Wait()
		select {
		case errChan <- fmt.Errorf("dail all maddr fail %s\n", errRes):
		case <-time.After(time.Millisecond * 500):
			return
		}
	}()

	for _, addr := range mas {
		go func(addr multiaddr.Multiaddr) {
			defer wg.Done()
			d := &mnet.Dialer{}
			if conn, err := d.DialContext(ctx, addr); err == nil {
				select {
				case connChan <- conn:
				case <-time.After(time.Second * 30):
				}
			} else {
				errRes = err.Error()
				if hst.cfg.Debug {
					log.Println("conn error:", err)
				}
			}
		}(addr)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("conn ctx quit")
		case conn := <-connChan:
			return conn, nil
		case err := <-errChan:
			return nil, err
		}
	}
}

// ConnectAddrStrings 连接字符串地址
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

// SendMsg 发送消息
func (hst *host) SendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) ([]byte, error) {
	clt, ok := hst.ClientStore().GetClient(pid)
	if !ok {
		return nil, fmt.Errorf("no client ID is:%s", pid.Pretty())
	}
	res, err := clt.SendMsg(ctx, mid, msg)
	return res, err
}
