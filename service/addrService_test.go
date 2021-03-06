package service_test

import (
	"crypto/rand"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	ythost "github.com/yottachain/YTHost"
	"github.com/yottachain/YTHost/service"
	"testing"
)

func TestAddrService_RemotePeerInfo(t *testing.T) {
	addrService := new(service.AddrService)
	pi, _, _ := ic.GenerateSecp256k1Key(rand.Reader)
	id, _ := peer.IDFromPrivateKey(pi)
	hst, _ := ythost.NewHost()
	addrService.Info.ID = id
	addrService.Info.Addrs = hst.Addrs()
	addrService.PubKey = pi.GetPublic()

	peeri := new(service.PeerInfo)

	_ = addrService.RemotePeerInfo("", peeri)

	t.Log(peeri)
}
