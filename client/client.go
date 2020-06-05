package client

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
	"net/rpc"
	"time"
)

type YTHostClient struct {
	*rpc.Client
	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	isClosed        bool
}

func (yc *YTHostClient) RemotePeer() peer.AddrInfo {
	var pi service.PeerInfo
	var ai peer.AddrInfo

	if err := yc.Call("as.RemotePeerInfo", "", &pi); err != nil {
		fmt.Println(err)
	}
	ai.ID = pi.ID
	for _, addr := range pi.Addrs {
		ma, _ := multiaddr.NewMultiaddr(addr)
		ai.Addrs = append(ai.Addrs, ma)
	}

	return ai
}

func (yc *YTHostClient) RemotePeerPubkey() crypto.PubKey {
	var pi service.PeerInfo

	if err := yc.Call("as.RemotePeerInfo", "", &pi); err != nil {
		fmt.Println(err)
	}
	pk, _ := crypto.UnmarshalPublicKey(pi.PubKey)
	return pk
}

func (yc *YTHostClient) LocalPeer() peer.AddrInfo {
	pi := peer.AddrInfo{}
	pi.ID = yc.localPeerID
	for _, v := range yc.localPeerAddrs {
		ma, _ := multiaddr.NewMultiaddr(v)
		pi.Addrs = append(pi.Addrs, ma)
	}
	return pi
}

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey) (*YTHostClient, error) {
	var yc = new(YTHostClient)
	yc.Client = clt
	yc.localPeerID = pi.ID
	yc.localPeerPubKey, _ = pk.Raw()

	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	yc.isClosed = false

	return yc, nil
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	var startTime = time.Now()

	stat.Default.Add(0, 0, 0, 1)

	var rpid = yc.RemotePeer().ID

	if rpid != "" && (id == 0xCB05 || id == 0xe75c) {
		stat.Default.Wait.Add(rpid)
	}

	resChan := make(chan service.Response)
	errChan := make(chan error)

	defer func() {
		if rpid != "" && (id == 0xCB05 || id == 0xe75c) {
			stat.Default.Wait.Sub(rpid)
		}
		if err := recover(); err != nil {
			errChan <- err.(error)
		}
	}()

	go func() {

		defer func() {
			if id == 0xCB05 || id == 0xe75c {
				stat.DefaultSpeedCounter.Push(yc.RemotePeer().ID, time.Now().Sub(startTime))
			}
		}()

		var res service.Response

		pi := service.PeerInfo{yc.localPeerID, yc.localPeerAddrs, yc.localPeerPubKey}

		if err := yc.Call("ms.HandleMsg", service.Request{id, data, pi}, &res); err != nil {
			select {
			case errChan <- err:
			case <-ctx.Done():
				return
			}
		} else {
			select {
			case resChan <- res:
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		if id == 0xCB05 || id == 0xe75c {
			stat.Default.Add(0, 0, 1, 0)
		}
		return nil, fmt.Errorf("ctx time out")
	case rd := <-resChan:
		if id == 0xCB05 || id == 0xe75c {
			stat.Default.Add(1, 0, 0, 0)
		}
		return rd.Data, nil
	case err := <-errChan:
		if id == 0xCB05 || id == 0xe75c {
			stat.Default.Add(0, 1, 0, 0)
		}
		return nil, err
	}
}

func (yc *YTHostClient) Ping(ctx context.Context) bool {

	successChan := make(chan struct{})
	errorChan := make(chan struct{})

	defer func() {
		if err := recover(); err != nil {
			errorChan <- struct{}{}
		}
	}()

	go func() {
		var res string
		if err := yc.Call("ms.Ping", "ping", &res); err != nil {
			errorChan <- struct{}{}
		} else if string(res) != "pong" {
			errorChan <- struct{}{}
		}
		successChan <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return false
	case <-errorChan:
		return false
	case <-successChan:
		return true
	}
}

func (yc *YTHostClient) Close() error {
	yc.isClosed = true
	return yc.Client.Close()
}

func (yc *YTHostClient) IsClosed() bool {
	return yc.isClosed
}

func (yc *YTHostClient) SendMsgClose(ctx context.Context, id int32, data []byte) ([]byte, error) {
	defer yc.Close()
	return yc.SendMsg(ctx, id, data)
}
