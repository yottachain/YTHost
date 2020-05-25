package stat

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type WaitMap struct {
	pool map[peer.ID]uint64
	sync.RWMutex
}

func (wm *WaitMap) Add(id peer.ID) {
	wm.Lock()
	defer wm.Unlock()

	if i, ok := wm.pool[id]; ok {
		wm.pool[id] = i + 1
	} else {
		wm.pool[id] = 1
	}
}

func (wm *WaitMap) Sub(id peer.ID) {
	wm.Lock()
	defer wm.Unlock()

	if i, ok := wm.pool[id]; ok {
		if i-1 <= 0 {
			fmt.Println("删除")
			delete(wm.pool, id)
		} else {
			fmt.Println(i - 1)
			wm.pool[id] = i - 1
		}
	} else {
		fmt.Println(ok, len(wm.pool))
	}
}
func (wm *WaitMap) Get(id peer.ID) uint64 {
	wm.RLock()
	defer wm.RUnlock()

	return wm.pool[id]
}

func (wm *WaitMap) Len() int {
	return len(wm.pool)
}

type Stat struct {
	Success int64
	Error   int64
	CtxDone int64
	Wait    WaitMap
}

func (s *Stat) Reset() {
	atomic.StoreInt64(&s.Success, 0)
	atomic.StoreInt64(&s.Error, 0)
	atomic.StoreInt64(&s.CtxDone, 0)
}

func (s *Stat) Get() (int, int64, int64, int64) {
	return s.Wait.Len(), atomic.LoadInt64(&s.Success), atomic.LoadInt64(&s.Error), atomic.LoadInt64(&s.CtxDone)
}

func (s *Stat) Add(ss int64, e int64, c int64) {
	if ss != 0 {
		atomic.AddInt64(&s.Success, ss)
	}
	if e != 0 {
		atomic.AddInt64(&s.Error, e)
	}
	if c != 0 {
		atomic.AddInt64(&s.CtxDone, c)
	}
}

var Default = &Stat{
	Wait: WaitMap{pool: make(map[peer.ID]uint64)},
}

func init() {
	fl, _ := os.OpenFile("ythost.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	log.SetOutput(fl)
	go func() {
		for {
			<-time.After(time.Second * 10)
			v1, v2, v3, v4 := Default.Get()
			log.Printf("并发 %d,成功 %d,失败 %d, 超时 %d\n", v1, v2, v3, v4)
			Default.Reset()
		}
	}()
}
