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
	sync.RWMutex
}

func (cc *clientContainer) GetStatus() int {
	cc.RLock()
	defer cc.RUnlock()

	return cc.Status
}

func (cc *clientContainer) SetStatus(s int) {
	cc.Lock()
	defer cc.Unlock()

	cc.Status = s
}
func (cc *clientContainer) SetClient(clt *client.YTHostClient) {
	cc.Lock()
	defer cc.Unlock()

	cc.YTHostClient = clt
}

type ClientPool struct {
	host     hostInterface.Host
	pool     sync.Map
	Interval time.Duration
	peers    []*peer.AddrInfo
}

func (cp *ClientPool) connect(info *peer.AddrInfo) {
	ac, _ := cp.pool.LoadOrStore(info.ID.Pretty(), &clientContainer{})
	cc := ac.(*clientContainer)

	ctx, cancel := context.WithTimeout(context.Background(), cp.Interval*2)
	defer cancel()

	clt, err := cp.host.ClientStore().Get(ctx, info.ID, info.Addrs)
	if err != nil {
		cc.SetStatus(0)
	} else {
		cc.SetStatus(1)
	}

	cc.SetClient(clt)
}

func (cp *ClientPool) GetFreeClients() []string {

	var free []string

	for _, v := range cp.peers {
		ac, ok := cp.pool.Load(v.ID.Pretty())
		if ok {
			cc := ac.(*clientContainer)
			if cc.GetStatus() == 1 || cc.GetStatus() == 2 {
				free = append(free, v.ID.Pretty())
			}
		}
	}

	return free
}

func (cp *ClientPool) Get(id string) (*client.YTHostClient, error) {
	ac, ok := cp.pool.Load(id)
	if !ok {
		return nil, fmt.Errorf("not client")
	}
	cc := ac.(*clientContainer)

	if cc.Status != 1 && cc.Status != 2 {
		return nil, fmt.Errorf("cliet not availablc")
	} else {
		cc.SetStatus(2)
		return cc.YTHostClient, nil
	}
}

func (cp *ClientPool) Put(id string) {
	ac, ok := cp.pool.Load(id)
	if ok {
		cc := ac.(*clientContainer)
		if ok {
			cc.SetStatus(1)
		}
	}
}

func (cp *ClientPool) Check() {

	for _, v := range cp.peers {
		ac, ok := cp.pool.Load(v.ID.Pretty())
		if ok {
			cc := ac.(*clientContainer)
			if cc.GetStatus() == 1 {
				go func() {
					// 正在检测
					cc.Status = 4
					ctx, _ := context.WithTimeout(context.Background(), cp.Interval)
					if !cc.YTHostClient.Ping(ctx) {
						cc.Status = 0
					} else {
						cc.Status = 1
					}
				}()
			} else if cc.GetStatus() == 0 {
				cc.Status = 3 // 正在连接中
				go cp.connect(v)
			}
		}
	}
}

func NewPool(hst hostInterface.Host, peers []*peer.AddrInfo) *ClientPool {
	var cp = ClientPool{
		host:     hst,
		pool:     sync.Map{},
		Interval: time.Second * 3,
		peers:    peers,
	}

	for _, peer := range peers {
		if peer == nil {
			continue
		}
		fmt.Println(peer.ID, peer.Addrs)
		go cp.connect(peer)
	}

	// 周期检查连接
	go func() {
		for {
			go cp.Check()
			<-time.After(cp.Interval + time.Millisecond*500)
		}
	}()

	return &cp
}

func (cp *ClientPool) Close(id string) {
	ac, ok := cp.pool.Load(id)
	if ok {
		cc := ac.(*clientContainer)
		cc.YTHostClient.Close()
		cc.SetStatus(0)
	}
}
