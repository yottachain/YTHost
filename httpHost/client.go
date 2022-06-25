package httpHost

import (
	"bytes"
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"io/ioutil"
	"net/http"
)

type Client struct {
	*http.Client
	localPeerInfo  peer.AddrInfo
	remotePeerInfo peer.AddrInfo
}

func (clt *Client) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	return clt.SendHTTPMsg(clt.remotePeerInfo.ID, clt.remotePeerInfo.Addrs[0], id, data)
}

func (clt *Client) SendHTTPMsg(id peer.ID, ma multiaddr.Multiaddr, mid int32, msg []byte) ([]byte, error) {
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
	resp, err := clt.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)
	return respData, err
}

func (clt *Client) RemotePeer() peer.AddrInfo {
	return clt.remotePeerInfo
}
func (clt *Client) LocalPeer() peer.AddrInfo {
	return clt.localPeerInfo
}

func (clt *Client) RemotePeerPubkey() crypto.PubKey {
	return nil
}

func NewClient(localPeer peer.AddrInfo, remotePeer peer.AddrInfo) *Client {
	return &Client{
		&http.Client{},
		localPeer,
		remotePeer,
	}
}
