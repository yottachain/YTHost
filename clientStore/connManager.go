package clientStore

import (
	"context"
	"fmt"
	"io"
	"net/rpc"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/yottachain/YTHost/client"
)

type ClientStore struct {
	connect func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
	sync.Map
	sync.Mutex
	IdLockMap map[peer.ID]chan int
}

func (cs *ClientStore) GetUsePid(pid peer.ID) *client.YTHostClient {
	_c, ok := cs.Map.Load(pid)
	if ok {
		return _c.(*client.YTHostClient)
	} else {
		return nil
	}
}

func (cs *ClientStore) BackConnect(pid peer.ID, addrs []string) {
	var mas = make([]multiaddr.Multiaddr, len(addrs))
	for k, v := range addrs {
		ma, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			continue
		}
		mas[k] = ma
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(10))
	defer cancel()
	_, _ = cs.Get(ctx, pid, mas)
}

func (cs *ClientStore) Get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	if _c, ok := cs.Map.Load(pid); ok {
		return _c.(*client.YTHostClient), nil
	}
	return cs.get(ctx, pid, mas)
}

func (cs *ClientStore) get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	cs.Lock() //len(IdLockMap)需要控制，定期清理
	idLock, ok := cs.IdLockMap[pid]
	if !ok {
		idLock = make(chan int, 1)
		cs.IdLockMap[pid] = idLock
	}
	cs.Unlock()

	select {
	case idLock <- 1:
		defer func() { <-idLock }()
		_c, ok := cs.Map.Load(pid)
		if !ok {
			if clt, err := cs.connect(ctx, pid, mas); err != nil {
				return nil, err
			} else {
				clt.ConnMap = cs.Map
				cs.Map.Store(pid, clt)
				return clt, nil
			}
		} else {
			return _c.(*client.YTHostClient), nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:waiting to connect")
	}
}

func (cs *ClientStore) GetByAddrString(ctx context.Context, id string, addrs []string) (*client.YTHostClient, error) {
	buf, _ := base58.Decode(id)
	pid, err := peer.IDFromBytes(buf)
	if err != nil {
		return nil, err
	}
	var mas = make([]multiaddr.Multiaddr, len(addrs))
	for k, v := range addrs {
		ma, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			continue
		}
		mas[k] = ma
	}
	return cs.get(ctx, pid, mas)
}

func (cs *ClientStore) Close(pid peer.ID) error {
	_clt, ok := cs.Load(pid)
	if !ok {
		return fmt.Errorf("no find client ID is %s", pid.Pretty())
	}
	clt := _clt.(*client.YTHostClient)
	return clt.Close()
}

func (cs *ClientStore) GetClient(pid peer.ID) (*client.YTHostClient, bool) {
	_clt, ok := cs.Load(pid)
	if ok {
		clt := _clt.(*client.YTHostClient)
		return clt, ok
	}
	return nil, ok
}

func (cs *ClientStore) PongDetect() {
	needping := make(map[*client.YTHostClient]bool)
	count := 0
	f := func(k, v interface{}) bool {
		count++
		c := v.(*client.YTHostClient)
		if c.IsconnTimeOut() && !c.IsUsed() {
			logrus.Infof("[ClientStore]No message sent in INTERVAL pid:%s\n", k.(peer.ID))
			c.Close()
			return true
		}
		if !c.IsUsed() {
			logrus.Tracef("[ClientStore]Need to PongDetect:%s\n", k.(peer.ID))
			needping[c] = true
		}
		return true
	}
	cs.Map.Range(f)
	logrus.Debugf("[ClientStore]Current connections: %d\n", count)
	for i := 0; i < 3; i++ {
		size := len(needping)
		if size == 0 {
			break
		}
		waitpong := make(map[*rpc.Call]*client.YTHostClient)
		done := make(chan *rpc.Call, size)
		for c := range needping {
			sendctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			call, err := c.SendPing(sendctx, done)
			if err == nil {
				waitpong[call] = c
			}
			cancel()
		}
		size = len(waitpong)
		if size == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	Loop:
		for ii := 0; ii < size; ii++ {
			select {
			case call := <-done:
				c := waitpong[call]
				if call.Error != nil {
					if call.Error == rpc.ErrShutdown || call.Error == io.ErrUnexpectedEOF {
						delete(needping, c)
						c.Close()
					}
					logrus.Infof("[ClientStore]Heartbeat ping fail:%s,pid:%s\n", call.Error, c.RemotePeer().ID)
				} else {
					res := call.Reply.(*string)
					if *res != "pong" {
						logrus.Infof("[ClientStore]Heartbeat ping %d times fail pid:%s\n", i+1, c.RemotePeer().ID)
					} else {
						delete(needping, c)
					}
				}
			case <-ctx.Done():
				logrus.Infoln("[ClientStore]Heartbeat ping timeout")
				break Loop
			}
		}
		cancel()
	}
	for c := range needping {
		logrus.Infof("[ClientStore]Heartbeat ping fail,close it,pid:%s\n", c.RemotePeer().ID)
		c.Close()
	}
}

func NewClientStore(connFunc func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)) *ClientStore {
	cs := &ClientStore{
		connFunc,
		sync.Map{},
		sync.Mutex{},
		make(map[peer.ID]chan int),
	}
	go func() {
		for {
			time.Sleep(time.Millisecond * time.Duration(client.PPI))
			cs.PongDetect()
		}
	}()
	return cs
}
