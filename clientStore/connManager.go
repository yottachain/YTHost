package clientStore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/client"
)

type ClientStore struct {
	connect  func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
	connects map[peer.ID]*client.YTHostClient
	sync.RWMutex
	IdLockMap map[peer.ID]chan int
}

func (cs *ClientStore) GetUsePid(pid peer.ID) *client.YTHostClient {
	cs.RLock()
	defer cs.RUnlock()
	return cs.connects[pid]
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
	cs.Get(ctx, pid, mas)
}

func (cs *ClientStore) Get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	cs.RLock()
	defer cs.RUnlock()
	if c, ok := cs.connects[pid]; ok {
		return c, nil
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
		c, ok := cs.GetClient(pid)
		if !ok {
			if clt, err := cs.connect(ctx, pid, mas); err != nil {
				return nil, err
			} else {
				clt.Remover = cs.DelClient
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
	clt, ok := cs.GetClient(pid)
	if !ok {
		return fmt.Errorf("no find client ID is %s", pid.Pretty())
	}
	return clt.Close()
}

func (cs *ClientStore) DelClient(pid peer.ID) {
	cs.Lock()
	defer cs.Unlock()
	delete(cs.connects, pid)
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

func NewClientStore(connFunc func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)) *ClientStore {
	cs := &ClientStore{
		connect:   connFunc,
		connects:  make(map[peer.ID]*client.YTHostClient),
		IdLockMap: make(map[peer.ID]chan int),
	}
	return cs
}
