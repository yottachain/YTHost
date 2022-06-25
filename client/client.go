package client

import (
	"context"
	"fmt"
	"io"
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
	Version         int32

	RPI *service.PeerInfo
	Cs  *stat.ConnStat

	isClosed     bool
	lastSendTime atomic.Value

	ConnMap sync.Map
	sync.Mutex
}

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, v int32, cs *stat.ConnStat) (*YTHostClient, error) {
	var yc = new(YTHostClient)
	yc.Client = clt
	yc.localPeerID = pi.ID
	yc.localPeerPubKey, _ = pk.Raw()
	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	yc.Version = v
	yc.RPI = new(service.PeerInfo)
	if err := yc.Call("as.RemotePeerInfo", "", yc.RPI); err != nil {
		return nil, err
	}

	yc.isClosed = false
	yc.lastSendTime.Store(time.Now().Unix())

	yc.Cs = cs
	return yc, nil
}

func (yc *YTHostClient) RemotePeer() peer.AddrInfo {
	var ai peer.AddrInfo
	ai.ID = yc.RPI.ID
	for _, addr := range yc.RPI.Addrs {
		ma, _ := multiaddr.NewMultiaddr(addr)
		ai.Addrs = append(ai.Addrs, ma)
	}
	return ai
}

func (yc *YTHostClient) RemotePeerPubkey() crypto.PubKey {
	pk, _ := crypto.UnmarshalPublicKey(yc.RPI.PubKey)
	return pk
}

func (yc *YTHostClient) RemotePeerVersion() int32 {
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
			if call.Error == rpc.ErrShutdown || call.Error == io.ErrUnexpectedEOF {
				yc.Close()
			}
			return nil, call.Error
		} else {
			return call.Reply.(*service.Response).Data, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out")
	}
}

func (yc *YTHostClient) SendPing(done chan *rpc.Call) *rpc.Call {
	return yc.Go("ms.Ping", "ping", new(string), done)
}

func (yc *YTHostClient) Close() error {
	yc.Lock()
	defer yc.Unlock()
	if yc.isClosed {
		return nil
	}
	yc.isClosed = true
	yc.ConnMap.Delete(yc.RemotePeer().ID)
	yc.Cs.CccSub()
	return yc.Client.Close()
}

func (yc *YTHostClient) IsClosed() bool {
	yc.Lock()
	defer yc.Unlock()
	return yc.isClosed
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
