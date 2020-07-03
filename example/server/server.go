package main

import (
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/newHost"
	"github.com/yottachain/YTHost/service"
)

func main(){
	maTCP,_ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9001")
	maHTTP,_ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9002/http")
	hp := newHost.NewHost([]multiaddr.Multiaddr{maTCP,maHTTP})
	hp.RegisterHandler(0x1c, func(requestData []byte, head service.Head) (bytes []byte, err error) {
		return []byte{111,222},nil
	})
	hp.Accept()
}
