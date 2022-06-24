package client

import (
	"context"
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

var PSI int
var PPI int

func init() {
	ssi := os.Getenv("P2P_SEND_MAX_INTERVAL")
	if ssi == "" {
		PSI = 600000
	} else {
		si, err := strconv.Atoi(ssi)
		if err != nil {
			PSI = 600000
		} else {
			PSI = si
		}
	}
	spi := os.Getenv("P2P_PING_INTERVAL")
	if spi == "" {
		PPI = 60000
	} else {
		pi, err := strconv.Atoi(spi)
		if err != nil {
			PPI = 60000
		} else {
			PPI = pi
		}
	}
}

type YTHostClient struct {
	*rpc.Client
	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	isClosed        atomic.Value
	lastSendTime    atomic.Value
	Version         int32
	RPI             *service.PeerInfo
	Cs              *stat.ConnStat
	Kill            func(c *YTHostClient)
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
	yc.Version = v
	yc.Cs = cs
	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	yc.isClosed.Store(false)
	return yc, nil
}

func (yc *YTHostClient) AsyncSendMsg(id int32, data []byte) *rpc.Call {
	yc.lastSendTime.Store(time.Now().Unix())
	req := service.Request{MsgId: id,
		ReqData: data,
		RemotePeerInfo: service.PeerInfo{ID: yc.localPeerID,
			Addrs:   yc.localPeerAddrs,
			PubKey:  yc.localPeerPubKey,
			Version: yc.Version},
	}
	return yc.Go("ms.HandleMsg", req, new(service.Response), make(chan *rpc.Call, 1))
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) (result []byte, e error) {
	call := yc.AsyncSendMsg(id, data)
	select {
	case <-call.Done:
		if call.Error != nil {
			yc.Kill(yc)
			return nil, call.Error
		} else {
			return call.Reply.(*service.Response).Data, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out")
	}
}

func (yc *YTHostClient) Ping(ctx context.Context) bool {
	call := yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
	select {
	case <-call.Done:
		if call.Error != nil {
			return false
		} else {
			res := call.Reply.(*string)
			if *res != "pong" {
				return false
			} else {
				return true
			}
		}
	case <-ctx.Done():
		return false
	}
}

func (yc *YTHostClient) Close() error {
	if yc.IsClosed() {
		return nil
	}
	yc.isClosed.Store(true)
	yc.Cs.CccSub()
	return yc.Client.Close()
}

func (yc *YTHostClient) IsClosed() bool {
	return yc.isClosed.Load().(bool)
}

func (yc *YTHostClient) SendMsgClose(ctx context.Context, id int32, data []byte) ([]byte, error) {
	defer yc.Close()
	return yc.SendMsg(ctx, id, data)
}

func (yc *YTHostClient) IsconnTimeOut() bool {
	t := yc.lastSendTime.Load().(int64)
	return time.Since(time.Unix(t, 0)).Milliseconds() > int64(PSI)
}

func (yc *YTHostClient) IsUsed() bool {
	t := yc.lastSendTime.Load().(int64)
	return !(time.Since(time.Unix(t, 0)).Milliseconds() > int64(PPI))
}
