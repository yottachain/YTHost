package clientStore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	"github.com/yottachain/YTHost/client"
)

type ClientStore struct {
	connect  func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
	connects map[peer.ID]*client.YTHostClient
	sync.RWMutex
	IdLockMap map[peer.ID]chan time.Time
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
	cs.Get(context.Background(), pid, mas)
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
	return cs.Get(ctx, pid, mas)
}

func (cs *ClientStore) Get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	if c, ok := cs.GetClient(pid); ok {
		return c, nil
	}
	cs.Lock()
	idLock, ok := cs.IdLockMap[pid]
	if !ok {
		idLock = make(chan time.Time, 1)
		idLock <- time.Unix(0, 0)
		cs.IdLockMap[pid] = idLock
	}
	cs.Unlock()
	if ctx == context.Background() {
		ctxcon, cancel := context.WithTimeout(ctx, time.Duration(client.GlobalClientOption.ConnectTimeout)*time.Millisecond)
		defer cancel()
		ctx = ctxcon
	}
	select {
	case state := <-idLock:
		defer func() { idLock <- state }()
		c, ok := cs.GetClient(pid)
		if !ok {
			if time.Since(state) < time.Duration(client.GlobalClientOption.ConnectTimeout)*time.Millisecond {
				return nil, fmt.Errorf("connection failed:retry frequently")
			}
			if clt, err := cs.connect(ctx, pid, mas); err != nil {
				state = time.Now()
				return nil, err
			} else {
				state = time.Unix(0, 0)
				clt.Start(func() {
					cs.Lock()
					defer cs.Unlock()
					delete(cs.connects, pid)
				})
				cs.AddClient(pid, clt)
				return clt, nil
			}
		} else {
			return c, nil
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx time out:waiting to connect")
	}
}

func (cs *ClientStore) Close(pid peer.ID) error {
	clt, ok := cs.GetClient(pid)
	if !ok {
		return fmt.Errorf("no find client ID is %s", pid.Pretty())
	}
	return clt.Close()
}

func (cs *ClientStore) AddClient(pid peer.ID, c *client.YTHostClient) {
	cs.Lock()
	defer cs.Unlock()
	cs.connects[pid] = c
}

func (cs *ClientStore) GetClient(pid peer.ID) (*client.YTHostClient, bool) {
	cs.RLock()
	defer cs.RUnlock()
	c, ok := cs.connects[pid]
	return c, ok
}

func (cs *ClientStore) GetConnections() int {
	cs.RLock()
	defer cs.RUnlock()
	return len(cs.connects)
}

func (cs *ClientStore) CheckDeadConnetion() {
	cs.RLock()
	var cons []*client.YTHostClient
	for _, c := range cs.connects {
		cons = append(cons, c)
	}
	cs.RUnlock()
	for _, c := range cons {
		if c.IsDazed() {
			c.Close()
		}
	}
	size := cs.GetConnections()
	if size > 0 {
		logrus.Tracef("[ClientStore]Current connections %d\n", size)
	} else {
		cs.Lock()
		defer cs.Unlock()
		cs.IdLockMap = make(map[peer.ID]chan time.Time)
	}
}

func NewClientStore(connFunc func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)) *ClientStore {
	cs := &ClientStore{
		connect:   connFunc,
		connects:  make(map[peer.ID]*client.YTHostClient),
		IdLockMap: make(map[peer.ID]chan time.Time),
	}
	go func() {
		for {
			time.Sleep(time.Millisecond * time.Duration(client.GlobalClientOption.MuteTimeout))
			cs.CheckDeadConnetion()
		}
	}()
	return cs
}
