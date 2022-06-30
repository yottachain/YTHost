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

var GlobalClientOption = &ClientOption{1, 5000, 5000, 10000, 15000, 60000 * 3, 60000}

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
	if ctx == context.Background() {
		ctxwrite, cancel := context.WithTimeout(ctx, time.Duration(GlobalClientOption.WriteTimeout)*time.Millisecond)
		defer cancel()
		ctx = ctxwrite
	}
	select {
	case ytcall.call = <-ytcall.writeDone:
		return nil
	case <-ctx.Done():
		atomic.AddInt32(&ytcall.cancel, 1)
		return fmt.Errorf("ctx time out:writing")
	}
}

func (ytcall *YTCall) Done(ctx context.Context) ([]byte, error) {
	if ctx == context.Background() {
		ctxread, cancel := context.WithTimeout(ctx, time.Duration(GlobalClientOption.ReadTimeout)*time.Millisecond)
		defer cancel()
		ctx = ctxread
	}
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

	Cs       *stat.ConnStat
	Remover  func()
	lastSend int64
}

func WarpClient(ctx context.Context, clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, v int32, cs *stat.ConnStat) (*YTHostClient, error) {
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
	infcall := yc.Go("as.RemotePeerInfo", "", yc.RPI, make(chan *rpc.Call, 1))
	select {
	case <-infcall.Done:
		if infcall.Error != nil {
			return nil, infcall.Error
		} else {
			break
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:getRemotePeerInfo")
	}
	return yc, nil
}

func (yc *YTHostClient) Start(remover func()) {
	yc.Remover = remover
	yc.Cs.CccAdd()
	go yc.DoSend()
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
			if yc.IsClosed() {
				return
			}
			lasttime = time.Now()
			atomic.StoreInt64(&yc.lastSend, lasttime.Unix())
			req.writeDone <- yc.Go("ms.HandleMsg", req.args, req.reply, make(chan *rpc.Call, 1))
			atomic.StoreInt64(&yc.lastSend, 0)
			pingerr = 0
			pingcall = nil
			lasttime = time.Now()
		case <-timer.C:
			if yc.IsClosed() || time.Since(lasttime).Milliseconds() > int64(GlobalClientOption.IdleTimeout) {
				yc.Close()
				return
			}
			if pingcall == nil {
				atomic.StoreInt64(&yc.lastSend, time.Now().Unix())
				pingcall = yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
				atomic.StoreInt64(&yc.lastSend, 0)
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
					logrus.Debugf("[HostClient]Peer %s is dead,shut it down\n", yc.RemotePeer().ID)
					yc.Close()
					return
				}
				atomic.StoreInt64(&yc.lastSend, time.Now().Unix())
				pingcall = yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
				atomic.StoreInt64(&yc.lastSend, 0)
			}
		}
		timer.Reset(time.Millisecond * time.Duration(GlobalClientOption.PingInterval))
	}
}

func (yc *YTHostClient) IsDazed() bool {
	lt := atomic.LoadInt64(&yc.lastSend)
	if lt > 0 && (time.Now().Unix()-lt)*1000 > int64(GlobalClientOption.WriteTimeout)*3 {
		return true
	}
	return false
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

func (yc *YTHostClient) CallRemotePeer(ctx context.Context) peer.AddrInfo {
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
	if ctx == context.Background() {
		ctxpush, cancel := context.WithTimeout(ctx, time.Duration(GlobalClientOption.QueueTimeout)*time.Millisecond)
		defer cancel()
		ctx = ctxpush
	}
	select {
	case yc.reqQueue <- msg:
		return msg, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:waiting to write")
	}
}

func (yc *YTHostClient) SendMsg(ctx context.Context, id int32, data []byte) ([]byte, error) {
	ytcall, err := yc.PushMsg(context.Background(), id, data)
	if err != nil {
		return nil, err
	}
	err = ytcall.WriteDone(context.Background())
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
	yc.Remover()
	yc.Cs.CccSub()
	err = yc.Client.Close()
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
