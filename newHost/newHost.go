package newHost

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	mnet "github.com/multiformats/go-multiaddr-net"
	host "github.com/yottachain/YTHost"
	"github.com/yottachain/YTHost/config"
	"github.com/yottachain/YTHost/httpHost"
	"github.com/yottachain/YTHost/interface"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/service"
	"sync"
)

type HostPool struct {
	cfg      *config.Config
	hosts    []YTinterface.Host
	addrbook sync.Map
}

func (hp *HostPool) RegisterHandler(id int32, handlerFunc service.Handler) error {
	for _, v := range hp.hosts {
		err := v.RegisterHandler(id, handlerFunc)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hp *HostPool) AddAddr(pid peer.ID, mas []multiaddr.Multiaddr) {
	hp.addrbook.Store(pid, mas)
}

func (hp *HostPool) SendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) ([]byte, error) {
	ac, ok := hp.addrbook.Load(pid)
	if !ok {
		return nil, fmt.Errorf("no peer addrInfo")
	}
	mas := ac.([]multiaddr.Multiaddr)

	for _, ma := range mas {

	}
	return nil, fmt.Errorf("send msg fail")
}

func (hp *HostPool) Addrs() []multiaddr.Multiaddr {

	port, err := hp.cfg.ListenAddr.ValueForProtocol(multiaddr.P_TCP)
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

func (hp *HostPool) Connect(ctx context.Context, pid peer.ID, ma multiaddr.Multiaddr) {
	if _, err := ma.ValueForProtocol(multiaddr.P_HTTP); err != nil {
		//clt := httpHost.NewClient(peer.AddrInfo{hp.cfg.ID,[]multiaddr.Multiaddr{}})
	}
}

func NewHost(mas []multiaddr.Multiaddr, opts ...option.Option) *HostPool {

	var hp HostPool
	hp.cfg = config.NewConfig()
	for _, bindOps := range opts {
		bindOps(hp.cfg)
	}

	var res []YTinterface.Host
	for _, ma := range mas {
		opts = append(opts, option.ListenAddr(ma))
		if _, err := ma.ValueForProtocol(multiaddr.P_HTTP); err == nil {

			hst, err := httpHost.NewHost(opts...)
			if err != nil {
				res = append(res, hst)
				hp.hosts = res
			}
		} else {
			hst, err := host.NewHost(opts...)
			if err != nil {
				res = append(res, hst)
				hp.hosts = res
			}
		}
	}

	return &hp
}
