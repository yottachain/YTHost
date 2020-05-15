package clientPool

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/hostInterface"
	"sync"
	"time"
)

type clientContainer struct {
	*client.YTHostClient
	Status int
	info   peer.AddrInfo
}

type ClientPool struct {
	host     hostInterface.Host
	pool     map[peer.ID]*clientContainer
	Interval time.Duration
	sync.Mutex
}

func (cp *ClientPool) connect(info peer.AddrInfo) {
	var cc clientContainer

	ctx, cancel := context.WithTimeout(context.Background(), cp.Interval*2)
	defer cancel()

	clt, err := cp.host.ClientStore().Get(ctx, info.ID, info.Addrs)
	if err != nil {
		cc.Status = 0
	} else {
		cc.Status = 1
	}

	cc.YTHostClient = clt
	cc.info = info

	cp.pool[info.ID] = &cc
}

func (cp *ClientPool) GetFreeClients() []peer.ID {
	cp.Lock()
	defer cp.Unlock()

	var free []peer.ID

	for k, v := range cp.pool {
		if v.Status == 1 {
			free = append(free, k)
		}
	}

	return free
}

func (cp *ClientPool) Get(id peer.ID) (*client.YTHostClient, error) {
	cp.Lock()
	defer cp.Lock()

	if item, ok := cp.pool[id]; !ok || item.Status != 1 {
		return nil, fmt.Errorf("cliet not available")
	} else {
		item.Status = 2
		cp.pool[id] = item

		return item.YTHostClient, nil
	}
}

func (cp *ClientPool) Put(id peer.ID) {
	cp.Lock()
	defer cp.Lock()

	item, ok := cp.pool[id]
	if ok {
		item.Status = 1
		cp.pool[id] = item
	}
}

func (cp *ClientPool) Check() {
	cp.Lock()
	defer cp.Unlock()

	for _, v := range cp.pool {
		if v.Status == 1 {
			go func() {
				ctx, _ := context.WithTimeout(context.Background(), cp.Interval)
				if !v.YTHostClient.Ping(ctx) {
					v.Status = 0
				}
			}()
		} else if v.Status == 0 {
			v.Status = 3 // 正在连接中
			go cp.connect(v.info)
		}
	}
}

func NewPool(hst hostInterface.Host, peers []peer.AddrInfo) *ClientPool {
	var cp = ClientPool{
		host:     hst,
		pool:     make(map[peer.ID]*clientContainer),
		Interval: time.Second * 3,
		Mutex:    sync.Mutex{},
	}

	for _, peer := range peers {
		go cp.connect(peer)
	}

	// 周期检查连接
	go func() {
		for {
			go cp.Check()
			<-time.After(cp.Interval)
		}
	}()

	return &cp
}
