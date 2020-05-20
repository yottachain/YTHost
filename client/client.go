package client

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/service"
	"net/rpc"
	"sync"
	"time"
)

type YTHostClient struct {
	*rpc.Client
	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	isClosed        bool
	WaitCount       uint64
	SuccessCount    uint64
	ErrorCount      uint64
	preDuration     time.Duration
	outTime         time.Duration
	sync.RWMutex
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
	yc.outTime = time.Second * 10

	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	yc.isClosed = false

	return yc, nil
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	yc.RLock()
	if yc.preDuration > 0 && yc.outTime > 0 && yc.WaitCount > 0 {
		if (time.Duration(yc.WaitCount) * yc.preDuration) > yc.outTime {
			return nil, fmt.Errorf("client wait queue overflow, len %d", yc.WaitCount)
		}
	}
	yc.RUnlock()

	startTime := time.Now()
	yc.Lock()
	yc.WaitCount++
	yc.Unlock()

	resChan := make(chan service.Response)
	errChan := make(chan error)

	defer func() {
		yc.Lock()
		yc.WaitCount--
		yc.preDuration = time.Now().Sub(startTime)
		yc.Unlock()

		err := recover()
		if err != nil {
			fmt.Println(err.(error).Error())
		}
	}()

	go func() {
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
		yc.Lock()
		yc.outTime = time.Now().Sub(startTime)
		yc.ErrorCount++
		yc.Unlock()

		return nil, fmt.Errorf("ctx time out")
	case rd := <-resChan:
		yc.Lock()
		yc.SuccessCount++
		yc.Unlock()

		return rd.Data, nil
	case err := <-errChan:
		yc.Lock()
		yc.ErrorCount--
		yc.Unlock()

		return nil, err
	}
}

func (yc *YTHostClient) Ping(ctx context.Context) bool {
	if yc == nil {
		return false
	}

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

//func init() {
//	fl, err := os.OpenFile("ythost.log", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
//	if err == nil {
//		log.SetOutput(fl)
//	}
//}
