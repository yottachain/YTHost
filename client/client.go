package client

import (
	"context"
	"fmt"
	"io"
	"net/rpc"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

var GlobalClientOption = &ClientOption{1, 5000, 2000, 3000, 10000, 60000 * 3, 30000}

type ClientOption struct {
	QueueSize      int
	ConnectTimeout int
	QueueTimeout   int
	WriteTimeout   int
	ReadTimeout    int

	IdleTimeout  int
	PingInterval int
}

type YTCall struct {
	client    *YTHostClient
	args      interface{}
	reply     interface{}
	writeDone chan *rpc.Call
	cancel    int32
	call      *rpc.Call
}

func (ytcall *YTCall) WriteDone(ctx context.Context) error {
	select {
	case ytcall.call = <-ytcall.writeDone:
		return nil
	case <-ctx.Done():
		atomic.AddInt32(&ytcall.cancel, 1)
		return fmt.Errorf("ctx time out:writing")
	}
}

func (ytcall *YTCall) Done(ctx context.Context) ([]byte, error) {
	if ytcall.call == nil {
		return nil, fmt.Errorf("message not sent")
	}
	select {
	case <-ytcall.call.Done:
		if ytcall.call.Error != nil {
			if ytcall.call.Error == rpc.ErrShutdown || ytcall.call.Error == io.ErrUnexpectedEOF {
				ytcall.client.Close()
			}
			return nil, ytcall.call.Error
		} else {
			return ytcall.call.Reply.(*service.Response).Data, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:waiting for response")
	}
}

type YTHostClient struct {
	*rpc.Client
	sync.Mutex

	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	Version         int32
	RPI             *service.PeerInfo

	reqQueue chan *YTCall
	isClosed bool

	Cs      *stat.ConnStat
	ConnMap sync.Map
}

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, v int32, cs *stat.ConnStat) (*YTHostClient, error) {
	yc := &YTHostClient{
		Client:      clt,
		localPeerID: pi.ID,
		Version:     v,
		RPI:         new(service.PeerInfo),
		isClosed:    false,
		Cs:          cs,
		reqQueue:    make(chan *YTCall, GlobalClientOption.QueueSize),
	}
	yc.localPeerPubKey, _ = pk.Raw()
	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	if err := yc.Call("as.RemotePeerInfo", "", yc.RPI); err != nil {
		return nil, err
	}
	go yc.DoSend()
	cs.CccAdd()
	logrus.Debugf("[HostClient]Successfully connected to %s,current connections %d\n", yc.RemotePeer().ID, yc.Cs.CliConnCount)
	return yc, nil
}

func (yc *YTHostClient) DoSend() {
	pingerr := 0
	lasttime := time.Now()
	var pingcall *rpc.Call = nil
	timer := time.NewTimer(time.Millisecond * time.Duration(GlobalClientOption.PingInterval))
	for {
		select {
		case req := <-yc.reqQueue:
			if atomic.LoadInt32(&req.cancel) > 0 {
				break
			}
			if yc.isClosed {
				return
			}
			req.writeDone <- yc.Go("ms.HandleMsg", req.args, req.reply, make(chan *rpc.Call, 1))
			pingerr = 0
			pingcall = nil
			lasttime = time.Now()
		case <-timer.C:
			if yc.isClosed || time.Since(lasttime).Milliseconds() > int64(GlobalClientOption.IdleTimeout) {
				yc.Close()
				return
			}
			if pingcall == nil {
				pingcall = yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
			} else {
				select {
				case call := <-pingcall.Done:
					if call.Error != nil {
						if call.Error == rpc.ErrShutdown || call.Error == io.ErrUnexpectedEOF {
							yc.Close()
							return
						}
						pingerr++
					} else {
						pingerr = 0
					}
				default:
					pingerr++
				}
				if pingerr >= 3 {
					yc.Close()
					return
				}
				pingcall = yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
			}
		}
		timer.Reset(time.Millisecond * time.Duration(GlobalClientOption.PingInterval))
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

func (yc *YTHostClient) PushMsg(ctx context.Context, id int32, data []byte) (*YTCall, error) {
	req := service.Request{MsgId: id,
		ReqData: data,
		RemotePeerInfo: service.PeerInfo{ID: yc.localPeerID,
			Addrs:   yc.localPeerAddrs,
			PubKey:  yc.localPeerPubKey,
			Version: yc.Version},
	}
	msg := &YTCall{args: req,
		reply:     new(service.Response),
		writeDone: make(chan *rpc.Call, 1),
		cancel:    0,
		client:    yc,
	}
	select {
	case yc.reqQueue <- msg:
		return msg, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:waiting to write")
	}
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	ctx_push, cancel_push := context.WithTimeout(context.Background(), time.Duration(GlobalClientOption.QueueTimeout)*time.Millisecond)
	defer cancel_push()
	ytcall, err := yc.PushMsg(ctx_push, id, data)
	if err != nil {
		return nil, err
	}
	ctx_write, cancel_write := context.WithTimeout(context.Background(), time.Duration(GlobalClientOption.WriteTimeout)*time.Millisecond)
	defer cancel_write()
	err = ytcall.WriteDone(ctx_write)
	if err != nil {
		return nil, err
	}
	return ytcall.Done(ctx)
}

func (yc *YTHostClient) Close() (err error) {
	yc.Lock()
	defer yc.Unlock()
	if yc.isClosed {
		return nil
	}
	yc.isClosed = true
	yc.ConnMap.Delete(yc.RemotePeer().ID)
	yc.Cs.CccSub()
	err = yc.Client.Close()
	logrus.Debugf("[HostClient]Disconnect %s,current connections %d\n", yc.RemotePeer().ID, yc.Cs.CliConnCount)
	return
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
