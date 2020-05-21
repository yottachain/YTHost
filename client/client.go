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
	s, ok := stat.DefaultStatTable.GetOrPut(yc.RemotePeer().ID, &stat.ClientStat{Outtime: time.Second * 10, PreRequestTime: time.Now()})
	if ok {
		s.RLock()
		// 如果等待队列长度大于 * 0.8(系数) 请求处理速度 返回失败
		if s.RequestHandleSpeed > 0 && s.RequestHandleSpeed*10 < s.Wait*8 {
			return nil, fmt.Errorf("[ythost] wait queue overflow len %d\n", s.Wait)
		}
		s.RUnlock()
	}

	s.RLock()
	outtime := s.Outtime
	s.RUnlock()

	// 这里为了方便计算 统计周期和超时时间一致
	if time.Now().Sub(time.Now()) > outtime {
		s.Lock()
		s.RequestHandleSpeed = s.Success

		s.PreRequestTime = time.Now()
		s.Success = 0
		s.Unlock()
	}

	startTime := time.Now()

	s.Lock()
	s.Wait++
	s.Unlock()

	resChan := make(chan service.Response)
	errChan := make(chan error)

	defer func() {

		err := recover()
		if err != nil {
			fmt.Println(err.(error).Error())
		}
	}()

	go func() {
		defer func() {
			s.Lock()
			s.Wait--
			s.Success++
			s.Unlock()
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
		s.Lock()
		s.Outtime = time.Now().Sub(startTime)
		s.Unlock()
		return nil, fmt.Errorf("ctx time out")
	case rd := <-resChan:
		return rd.Data, nil
	case err := <-errChan:
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
