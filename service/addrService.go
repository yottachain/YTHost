package service

import (
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

type AddrService struct {
	Info    peer.AddrInfo
	PubKey  crypto.PubKey
	Version int32
}

type PeerInfo struct {
	ID     peer.ID
	Addrs  []string
	PubKey []byte
	Version int32
}

func (as *AddrService) RemotePeerInfo(req string, res *PeerInfo) error {
	res.ID = as.Info.ID
	pk, err := crypto.MarshalPublicKey(as.PubKey)
	if err != nil {
		return err
	}
	res.PubKey = pk
	//res.Addrs = as.Info.Addrs
	for _, addr := range as.Info.Addrs {
		res.Addrs = append(res.Addrs, addr.String())
	}
	res.Version = as.Version
	return nil
}
