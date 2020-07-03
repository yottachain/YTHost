package main

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	host "github.com/yottachain/YTHost"
)

func main()  {
	hst,_:=host.NewHost()
	ma,_ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9002/http")
	res,err:=hst.SendMsgAuto(context.Background(),peer.ID("1111"),0x1c,ma,[]byte{22,33})
	fmt.Println(res,err)
}
