package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/service"
	"github.com/yottachain/YTHost/stat"
)

var QueueSize int = 2
var ConnectTimeout int = 5000
var QueueTimeout int = 5000
var WriteTimeout int = 10000
var ReadTimeout int = 15000
var MuteTimeout int = 60000
var IdleTimeout int = 60000 * 3

type YTConn struct {
	net.Conn
	ActiveTime *int64
}

func (conn *YTConn) Write(buf []byte) (int, error) {
	n, err := conn.Conn.Write(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		atomic.StoreInt64(conn.ActiveTime, time.Now().Unix())
	}
	return n, err
}

func (conn *YTConn) Read(buf []byte) (int, error) {
	n, err := conn.Conn.Read(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		atomic.StoreInt64(conn.ActiveTime, time.Now().Unix())
	}
	return n, err
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
		ctxwrite, cancel := context.WithTimeout(ctx, time.Duration(WriteTimeout)*time.Millisecond)
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

func (ytcall *YTCall) ReadDone(ctx context.Context) ([]byte, error) {
	if ctx == context.Background() {
		ctxread, cancel := context.WithTimeout(ctx, time.Duration(ReadTimeout)*time.Millisecond)
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

	Cs         *stat.ConnStat
	Remover    func()
	activeTime *int64
}

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, v int32, cs *stat.ConnStat, aread *int64) *YTHostClient {
	yc := &YTHostClient{
		Client:      clt,
		localPeerID: pi.ID,
		Version:     v,
		isClosed:    false,
		Cs:          cs,
		reqQueue:    make(chan *YTCall, QueueSize),
		activeTime:  aread,
	}
	yc.localPeerPubKey, _ = pk.Raw()
	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	return yc
}

func (yc *YTHostClient) Start(remover func()) {
	yc.Remover = remover
	yc.Cs.CccAdd()
	go yc.DoSend()
}

func (yc *YTHostClient) DoSend() {
	lasttime := time.Now()
	timer := time.NewTimer(time.Millisecond * time.Duration(WriteTimeout))
	for {
		select {
		case req := <-yc.reqQueue:
			if atomic.LoadInt32(&req.cancel) > 0 {
				break
			}
			if yc.IsClosed() {
				return
			}
			req.writeDone <- yc.Go("ms.HandleMsg", req.args, req.reply, make(chan *rpc.Call, 1))
			lasttime = time.Now()
		case <-timer.C:
			if yc.IsClosed() || yc.IsDazed() || time.Since(lasttime).Milliseconds() > int64(IdleTimeout) {
				yc.Close()
				return
			}
			yc.Go("ms.Ping", "ping", new(string), make(chan *rpc.Call, 1))
		}
		timer.Reset(time.Millisecond * time.Duration(WriteTimeout))
	}
}

func (yc *YTHostClient) IsDazed() bool {
	rwt := atomic.LoadInt64(yc.activeTime)
	if rwt > 0 && (time.Now().Unix()-rwt)*1000 > int64(MuteTimeout) {
		return true
	}
	return false
}

func (yc *YTHostClient) RemotePeerInfo() (*service.PeerInfo, error) {
	info := new(service.PeerInfo)
	yc.Lock()
	defer yc.Unlock()
	if yc.RPI == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ConnectTimeout)*time.Millisecond)
		defer cancel()
		infcall := yc.Go("as.RemotePeerInfo", "", info, make(chan *rpc.Call, 1))
		select {
		case <-infcall.Done:
			if infcall.Error != nil {
				return nil, infcall.Error
			} else {
				yc.RPI = info
				return yc.RPI, nil
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("ctx time out:getRemotePeerInfo")
		}
	} else {
		return yc.RPI, nil
	}
}

func (yc *YTHostClient) RemotePeer() peer.AddrInfo {
	var ai peer.AddrInfo
	if info, err := yc.RemotePeerInfo(); err == nil {
		ai.ID = info.ID
		for _, addr := range info.Addrs {
			ma, _ := multiaddr.NewMultiaddr(addr)
			ai.Addrs = append(ai.Addrs, ma)
		}
	}
	return ai
}

func (yc *YTHostClient) RemotePeerPubkey() crypto.PubKey {
	if info, err := yc.RemotePeerInfo(); err == nil {
		if pk, er := crypto.UnmarshalPublicKey(info.PubKey); er == nil {
			return pk
		}
	}
	return nil
}

func (yc *YTHostClient) RemotePeerVersion() int32 {
	if info, err := yc.RemotePeerInfo(); err == nil {
		return info.Version
	}
	return 0
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
		ctxpush, cancel := context.WithTimeout(ctx, time.Duration(QueueTimeout)*time.Millisecond)
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
	return ytcall.ReadDone(ctx)
}

func (yc *YTHostClient) Close() (err error) {
	yc.Lock()
	defer yc.Unlock()
	if yc.isClosed {
		return nil
	}
	yc.isClosed = true
	if yc.Remover != nil {
		yc.Remover()
	}
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
