package httpHost

import (
	"bytes"
	"context"
	"fmt"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/stat"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	_ "net/http/pprof"
	"net/rpc"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/multiformats/go-multiaddr"

	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/config"
	"github.com/yottachain/YTHost/service"
)

type host struct {
	cfg       *config.Config
	listenner manet.Listener
	client    *http.Client
	//transport *Transport
	sync.Map
	Cs *stat.ConnStat
}

func (h *host) Accept() {
	hlis := manet.NetListener(h.listenner)

	http.Serve(hlis, nil)
}

func (h *host) Addrs() []multiaddr.Multiaddr {
	port, err := h.listenner.Multiaddr().ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return nil
	}

	tcpMa, err := multiaddr.NewMultiaddr(fmt.Sprintf("/tcp/%s", port))

	if err != nil {
		return nil
	}

	var res []multiaddr.Multiaddr
	maddrs, err := manet.InterfaceMultiaddrs()
	if err != nil {
		return nil
	}

	for _, ma := range maddrs {
		newMa := ma.Encapsulate(tcpMa)
		if manet.IsIPLoopback(newMa) {
			continue
		}
		res = append(res, newMa)
	}
	return res
}

func (h host) Server() *rpc.Server {
	panic("implement me")
}

func (h *host) Config() *config.Config {
	return h.cfg
}

func (hst *host) ConnStat() *stat.ConnStat {
	return hst.Cs
}

func (h *host) Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	h.Store(pid, mas)
	return nil, nil
}

func (h *host) RegisterHandler(id int32, handlerFunc service.Handler) error {
	h.registerHttpHandler(fmt.Sprintf("/msg/%d", id), handlerFunc, id)
	return nil
}

func (h *host) registerHttpHandler(p string, handlerFunc service.Handler, id int32) {
	http.HandleFunc(p, func(writer http.ResponseWriter, request *http.Request) {

		reqData, err := ioutil.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprintln(writer, "request body read error:", err.Error())
			_, _ = writer.Write([]byte{})
			return
		}

		pk, err := h.cfg.Privkey.GetPublic().Raw()
		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprintln(writer, "get pubkey error:", err.Error())
			_, _ = writer.Write([]byte{})
			return
		}
		msgId := 0
		_, _ = fmt.Sscanf(request.URL.String(),"/msg/%d", &msgId)
		res, err := handlerFunc(reqData, service.Head{MsgId: int32(msgId), RemotePeerID: h.cfg.ID, RemoteAddrs: h.Addrs(), RemotePubKey: pk})
		if err != nil {
			writer.WriteHeader(500)
			_, _ = fmt.Fprintln(writer, err.Error())
		} else {
			_, _ = writer.Write(res)
		}

	})
}

func (h *host) RegisterGlobalMsgHandler(handlerFunc service.Handler) {
	h.registerHttpHandler("/msg/", handlerFunc, 0)
}

func (h *host) RemoveHandler(id int32) {
	panic("implement me")
}

func (h host) RemoveGlobalHandler() {
	panic("implement me")
}

func (h host) ConnectAddrStrings(ctx context.Context, id string, addrs []string) (*client.YTHostClient, error) {
	panic("implement me")
}

func (h host) ClientStore() *clientStore.ClientStore {
	panic("implement me")
}

func (h host) SendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) ([]byte, error) {
	panic("implement me")
}

func (hst *host)SendMsgAuto(ctx context.Context, pid peer.ID,mid int32,  ma multiaddr.Multiaddr,msg []byte) ([]byte,error) {
	panic("implement me")
}

func (h *host) SendHTTPMsg(id peer.ID, ma multiaddr.Multiaddr, mid int32, msg []byte) ([]byte, error) {
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
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%s/msg/%d", addr, port, mid), bytes.NewBuffer(msg))
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)

	return respData, err
}

func NewHost(options ...option.Option) (*host, error) {
	hst := new(host)
	hst.cfg = config.NewConfig()

	for _, bindOp := range options {
		bindOp(hst.cfg)
	}

	// 开启pprof
	if hst.cfg.PProf != "" {
		go func() {
			if err := http.ListenAndServe(hst.cfg.PProf, nil); err != nil {
				fmt.Println("PProf open fail:", err)
			} else {
				fmt.Println("PProf debug open:", hst.cfg.PProf)
			}
		}()
	}

	lis, err := manet.Listen(hst.cfg.ListenAddr)
	if err != nil {
		return nil, err
	}

	hst.listenner = lis

	tr := &http.Transport{
		IdleConnTimeout:    2 * time.Second,
	}

	hst.client = &http.Client{Transport: tr}
	hst.client.Timeout = time.Second * 30
	//hst.client.Transport = hst.transport

	return hst, nil
}
