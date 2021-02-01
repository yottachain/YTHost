package clientStore

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/client"
	"os"
	"strconv"
	"sync"
	"time"
)

var ppi int	//

func init() {
	spi := os.Getenv("P2P_PING_INTERVAL")
	//ppi = 60000
	if spi == "" {
		ppi = 20000
	}else {
		pi, err := strconv.Atoi(spi)
		if err != nil {
			ppi = 20000
		}else {
			ppi = pi
		}
	}
}

type ClientStore struct {
	connect func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
	q       chan struct{}
	sync.Map
	sync.Mutex
	IdLockMap map[peer.ID] *sync.Mutex
}

func (cs *ClientStore) GetUsePid(pid peer.ID) (c *client.YTHostClient){
	//cs.Lock()
	//defer cs.Unlock()
	_c, ok := cs.Map.Load(pid)
	if ok {
		c = _c.(*client.YTHostClient)
	}else {
		c = nil
	}
	return
}

func (cs *ClientStore) BackConnect (pid peer.ID, addrs []string) {
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

// Get 获取一个客户端，如果没有，建立新的客户端连接
func (cs *ClientStore) Get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx done")
	default:
		return cs.get(ctx, pid, mas)
	}
}

func (cs *ClientStore) get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	cs.q <- struct {}{}
	defer func() {
		<-cs.q
	}()

	// 尝试次数
	var tryCount int
	const max_try_count = 5

	cs.Lock()
	idLock, ok := cs.IdLockMap[pid]
	if !ok {
		cs.IdLockMap[pid] = &sync.Mutex{}
		idLock, _ = cs.IdLockMap[pid]
	}
	cs.Unlock()

	idLock.Lock()
	defer idLock.Unlock()

	// 取已存在clt
start:
	// 如果达到最大尝试次数就返回错误
	if tryCount++; tryCount > max_try_count {
		return nil, fmt.Errorf("Maximum attempts %d ", max_try_count)
	}
	_c, ok := cs.Map.Load(pid)
	// 如果不存在创建新的clt
	if !ok {
		if clt, err := cs.connect(ctx, pid, mas); err != nil {
			return nil, err
		} else {
			cs.Map.Store(pid, clt)
			// 创建clt完成后返回到开始
			goto start
		}
	} else {
		// 如果已存在clt无法ping通,删除记录重新创建
		c := _c.(*client.YTHostClient)
		if c.IsClosed() {
			cs.Map.Delete(pid)
			goto start
		}

		return c, nil
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

// Close 关闭一个客户端
func (cs *ClientStore) Close(pid peer.ID) error {
	cs.Lock()
	defer cs.Unlock()

	_clt, ok := cs.Load(pid)
	if !ok {
		return fmt.Errorf("no find client ID is %s", pid.Pretty())
	}
	clt := _clt.(*client.YTHostClient)

	cs.Map.Delete(pid)
	return clt.Close()
}

func (cs *ClientStore) GetClient(pid peer.ID) (*client.YTHostClient, bool) {
	_clt, ok := cs.Map.Load(pid)
	if ok {
		clt := _clt.(*client.YTHostClient)
		return clt, ok
	}
	return nil, ok
}

// Len 返回当前连接数
//func (cs *ClientStore) Len() int {
//}

func (cs *ClientStore) PongDetect() {
	wg := &sync.WaitGroup{}

	f := func(k, v interface{}) bool {
		c := v.(*client.YTHostClient)
		go func() {
			wg.Add(1)
			defer wg.Done()

			if c.IsconnTimeOut() && !c.IsUsed() {
				//fmt.Printf("No message sent in INTERVAL pid=%s\n", peer.Encode(k.(peer.ID)))
				cs.Lock()
				_ = c.Close()
				cs.Map.Delete(k.(peer.ID))
				cs.Unlock()
				return
			}

			//当前链接没有通信的情况才发送心跳，否则不用发送心跳检测
			var pstatus = true
			if !c.IsUsed() {
				var pongs = 2
				for i := 0; i < pongs; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					if !c.Ping(ctx) {
						//fmt.Printf("heartbeat ping %d times fail pid=%s\n", i+1, peer.Encode(k.(peer.ID)))
						pstatus = false
						cancel()
						<-time.After(100 * time.Millisecond)
					} else {
						//fmt.Printf("heartbeat ping %d times success pid=%s\n", i+1, peer.Encode(k.(peer.ID)))
						pstatus = true
						cancel()
						break
					}
				}
			}

			if !pstatus && !c.IsUsed() {
				//fmt.Printf("heartbeat ping fail pid=%s, connect close\n", peer.Encode(k.(peer.ID)))
				cs.Lock()
				_ = c.Close()
				cs.Map.Delete(k.(peer.ID))
				cs.Unlock()
				return
			}
		}()

		return true
	}

	for {
		wg = &sync.WaitGroup{}
		<- time.After(time.Duration(ppi)*time.Millisecond)
		cs.Map.Range(f)
		wg.Wait()
	}
}

func NewClientStore(connFunc func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)) *ClientStore {
	cs := &ClientStore{
		connFunc,
		make(chan struct{}, 50000),
		sync.Map{},
		sync.Mutex{},
		make(map[peer.ID] *sync.Mutex),
	}

	go cs.PongDetect()

	return cs
}
