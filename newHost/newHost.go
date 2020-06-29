package newHost

import (
	"github.com/multiformats/go-multiaddr"
	host "github.com/yottachain/YTHost"
	"github.com/yottachain/YTHost/httpHost"
	"github.com/yottachain/YTHost/interface"
	"github.com/yottachain/YTHost/option"
)

func NewHost(mas []multiaddr.Multiaddr, opts ...option.Option) (res []YTinterface.Host) {
	for _, ma := range mas {
		opts = append(opts, option.ListenAddr(ma))
		if _, err := ma.ValueForProtocol(multiaddr.P_HTTP); err == nil {

			hst, err := httpHost.NewHost(opts...)
			if err != nil {
				res = append(res, hst)
				return
			}
		} else {
			hst, err := host.NewHost(opts...)
			if err != nil {
				res = append(res, hst)
				return
			}
		}
	}

	return
}
