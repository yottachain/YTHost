package newHost

import (
	"sync"

	"github.com/multiformats/go-multiaddr"
	host "github.com/yottachain/YTHost"
	"github.com/yottachain/YTHost/httpHost"
	YTinterface "github.com/yottachain/YTHost/interface"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/service"
)

type HostPool struct {
	Hosts    []YTinterface.Host
	addrbook sync.Map
}

func (hp *HostPool) RegisterHandler(id int32, handlerFunc service.Handler) error {
	for _, v := range hp.Hosts {
		err := v.RegisterHandler(id, handlerFunc)
		if err != nil {
			return err
		}
	}
	return nil
}

func (hp *HostPool) RegisterGlobalMsgHandler(handlerFunc service.Handler) error {
	for _, v := range hp.Hosts {
		v.RegisterGlobalMsgHandler(handlerFunc)
	}
	return nil
}

func (hp *HostPool) Accept() {
	for _, v := range hp.Hosts {
		go v.Accept()
	}
}

func NewHost(mas []multiaddr.Multiaddr, opts ...option.Option) *HostPool {

	var hp HostPool

	var res []YTinterface.Host
	for _, ma := range mas {
		opts = append(opts, option.ListenAddr(ma))
		if _, err := ma.ValueForProtocol(multiaddr.P_HTTP); err == nil {

			hst, err := httpHost.NewHost(opts...)
			if err == nil {
				res = append(res, hst)
				hp.Hosts = res
			}
		} else {
			hst, err := host.NewHost(opts...)
			if err == nil {
				res = append(res, hst)
				hp.Hosts = res
			}
		}
	}

	return &hp
}
