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

type Message struct {
	serviceMethod string
	args          interface{}
	reply         interface{}
	done          chan *rpc.Call
	result        chan *rpc.Call
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
	waitWrite chan *Message
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
	yc.waitWrite = make(chan *Message, 1)
	go yc.WriteRequest()
	return yc, nil
}

func (yc *YTHostClient) WriteRequest() {
	for {
		req, ok := <-yc.waitWrite
		if !ok {
			return
		}
		req.result <- yc.Go(req.serviceMethod, req.args, req.reply, req.done)
		if req.serviceMethod == "ms.HandleMsg" {
			yc.lastSendTime.Store(time.Now().Unix())
		}
	}
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

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	call, err := yc.AsyncSendMsg(ctx, id, data)
	if err != nil {
		return nil, err
	}
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

func (yc *YTHostClient) AsyncSendMsg(ctx context.Context, id int32, data []byte) (*rpc.Call, error) {
	req := service.Request{MsgId: id,
		ReqData: data,
		RemotePeerInfo: service.PeerInfo{ID: yc.localPeerID,
			Addrs:   yc.localPeerAddrs,
			PubKey:  yc.localPeerPubKey,
			Version: yc.Version},
	}
	msg := &Message{serviceMethod: "ms.HandleMsg",
		args:  req,
		reply: new(service.Response),
		done:  make(chan *rpc.Call, 1),
	}
	return yc.writeMessage(ctx, msg)
}

func (yc *YTHostClient) writeMessage(ctx context.Context, msg *Message) (*rpc.Call, error) {
	msg.result = make(chan *rpc.Call, 1)
	select {
	case yc.waitWrite <- msg:
		select {
		case call := <-msg.result:
			return call, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("ctx time out")
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out")
	}
}

func (yc *YTHostClient) SendPing(ctx context.Context, done chan *rpc.Call) (*rpc.Call, error) {
	msg := &Message{serviceMethod: "ms.Ping",
		args:  "ping",
		reply: new(string),
		done:  done,
	}
	return yc.writeMessage(ctx, msg)
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
	close(yc.waitWrite)
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
