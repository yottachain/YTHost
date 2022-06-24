package client

import (
	"context"
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

var psi int

func init() {
	ssi := os.Getenv("P2P_SEND_MAX_INTERVAL")
	if ssi == "" {
		psi = 1800000
	} else {
		si, err := strconv.Atoi(ssi)
		if err != nil {
			psi = 1800000
		} else {
			psi = si
		}
	}
}

type YTHostClient struct {
	*rpc.Client
	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	isClosed        bool
	lastSendTime    atomic.Value
	uses            int32
	Version         int32
	RPI             *service.PeerInfo
	Cs              *stat.ConnStat
	sync.Mutex
}

func (yc *YTHostClient) GetRPI() error {
	var pi service.PeerInfo
	if err := yc.Call("as.RemotePeerInfo", "", &pi); err != nil {
		return err
	}
	yc.RPI = &pi
	return nil
}

func (yc *YTHostClient) RemotePeer() peer.AddrInfo {
	var ai peer.AddrInfo
	if yc.RPI == nil {
		err := yc.GetRPI()
		if err != nil {
			fmt.Println(err)
		}
	}

	ai.ID = yc.RPI.ID
	for _, addr := range yc.RPI.Addrs {
		ma, _ := multiaddr.NewMultiaddr(addr)
		ai.Addrs = append(ai.Addrs, ma)
	}

	return ai
}

func (yc *YTHostClient) RemotePeerPubkey() crypto.PubKey {
	if yc.RPI == nil {
		err := yc.GetRPI()
		if err != nil {
			fmt.Println(err)
		}
	}
	pk, _ := crypto.UnmarshalPublicKey(yc.RPI.PubKey)
	return pk
}

func (yc *YTHostClient) RemotePeerVersion() int32 {
	if yc.RPI == nil {
		err := yc.GetRPI()
		if err != nil {
			fmt.Println(err)
			return 0
		}
	}
	return yc.RPI.Version
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

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, v int32, cs *stat.ConnStat) (*YTHostClient, error) {
	var yc = new(YTHostClient)
	yc.Client = clt
	yc.localPeerID = pi.ID
	yc.localPeerPubKey, _ = pk.Raw()
	yc.lastSendTime.Store(time.Now().Unix())
	yc.uses = 0
	yc.Version = v
	yc.Cs = cs

	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}

	yc.Lock()
	yc.isClosed = false
	yc.Unlock()

	return yc, nil
}

func (yc *YTHostClient) SendMsgBlock(id int32, data []byte) ([]byte, error) {
	atomic.AddInt32(&yc.uses, 1)
	defer atomic.AddInt32(&yc.uses, -1)
	var res service.Response
	pi := service.PeerInfo{yc.localPeerID, yc.localPeerAddrs, yc.localPeerPubKey, yc.Version}
	yc.lastSendTime.Store(time.Now().Unix())
	err := yc.Call("ms.HandleMsg", service.Request{id, data, pi}, &res)
	if err != nil {
		return nil, err
	} else {
		return res.Data, nil
	}
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) (result []byte, e error) {
	defer func() {
		atomic.AddInt32(&yc.uses, -1)
		if err := recover(); err != nil {
			e = err.(error)
		}
	}()
	yc.lastSendTime.Store(time.Now().Unix())
	atomic.AddInt32(&yc.uses, 1)
	pi := service.PeerInfo{yc.localPeerID, yc.localPeerAddrs, yc.localPeerPubKey, yc.Version}
	call := yc.Go("ms.HandleMsg", service.Request{id, data, pi}, new(service.Response), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		if call.Error != nil {
			return nil, call.Error
		} else {
			return call.Reply.(*service.Response).Data, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out")
	}
}

func (yc *YTHostClient) SendMsg2(ctx context.Context, id int32, data []byte) ([]byte, error) {

	resChan := make(chan service.Response)
	errChan := make(chan error)

	defer func() {
		if err := recover(); err != nil {
			errChan <- err.(error)
		}
		atomic.AddInt32(&yc.uses, -1)
	}()
	yc.lastSendTime.Store(time.Now().Unix())
	atomic.AddInt32(&yc.uses, 1)

	go func() {
		var res service.Response
		errC := make(chan error, 1)
		pi := service.PeerInfo{yc.localPeerID, yc.localPeerAddrs, yc.localPeerPubKey, yc.Version}

		select {
		case errC <- yc.Call("ms.HandleMsg", service.Request{id, data, pi}, &res):
			err := <-errC
			if nil != err {
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
		case <-ctx.Done():
			return
		}
		//if err := yc.Call("ms.HandleMsg", service.Request{id, data, pi}, &res); err != nil {
		//	select {
		//	case errChan <- err:
		//	case <-ctx.Done():
		//		return
		//	}
		//} else {
		//	select {
		//	case resChan <- res:
		//	case <-ctx.Done():
		//		return
		//	}
		//}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out")
	case rd := <-resChan:
		return rd.Data, nil
	case err := <-errChan:
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
		var errC = make(chan error, 1)
		select {
		case errC <- yc.Call("ms.Ping", "ping", &res):
			err := <-errC
			if err != nil {
				select {
				case errorChan <- struct{}{}:
				default:
				}
			} else if string(res) != "pong" {
				select {
				case errorChan <- struct{}{}:
				default:
				}
			} else {
				select {
				case successChan <- struct{}{}:
				default:
				}
			}
		case <-ctx.Done():
			select {
			case errorChan <- struct{}{}:
			default:
			}
		}
		//if err := yc.Call("ms.Ping", "ping", &res); err != nil {
		//	select {
		//	case errorChan <- struct{}{}:
		//	default:
		//	}
		//} else if string(res) != "pong" {
		//	select {
		//	case errorChan <- struct{}{}:
		//	default:
		//	}
		//
		//} else {
		//	select {
		//	case successChan <- struct{}{}:
		//	default:
		//	}
		//}
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
	yc.Lock()
	defer yc.Unlock()
	if yc.isClosed {
		return nil
	}
	yc.isClosed = true
	yc.Cs.CccSub()
	return yc.Client.Close()
}

func (yc *YTHostClient) IsClosed() bool {
	//yc.Lock()
	//defer yc.Unlock()
	return yc.isClosed
}

func (yc *YTHostClient) SendMsgClose(ctx context.Context, id int32, data []byte) ([]byte, error) {
	defer yc.Close()
	return yc.SendMsg(ctx, id, data)
}

func (yc *YTHostClient) IsconnTimeOut() bool {
	obj := yc.lastSendTime.Load()
	t, _ := obj.(int64)
	if time.Now().Sub(time.Unix(t, 0)).Milliseconds() > int64(psi) {
		return true
	} else {
		return false
	}
}

func (yc *YTHostClient) IsUsed() bool {
	if atomic.LoadInt32(&yc.uses) == 0 {
		return false
	} else {
		return true
	}
}
