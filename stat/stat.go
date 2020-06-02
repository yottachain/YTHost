package stat

import (
	"bytes"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type OPMsg struct {
	OP    string
	Value uint64
}

type WaitMap struct {
	pool map[peer.ID]uint64
	Sum  int64
	sync.RWMutex
}

func (wm *WaitMap) Add(id peer.ID) {
	atomic.AddInt64(&wm.Sum, 1)

	wm.RLock()
	i, ok := wm.pool[id]
	wm.RUnlock()

	wm.Lock()
	if ok {
		wm.pool[id] = i + 1
	} else {
		wm.pool[id] = 1
	}
	wm.Unlock()
}

func (wm *WaitMap) Sub(id peer.ID) {
	atomic.AddInt64(&wm.Sum, 1)

	wm.RLock()
	i, ok := wm.pool[id]
	wm.RUnlock()

	if ok {
		wm.Lock()
		if i-1 <= 0 {
			delete(wm.pool, id)
		} else {
			wm.pool[id] = i - 1
		}
		wm.Unlock()
	} else {
		fmt.Println(ok, len(wm.pool))
	}
}
func (wm *WaitMap) Get(id peer.ID) uint64 {
	wm.RLock()
	defer wm.RUnlock()
	res, ok := wm.pool[id]
	if ok {
		return res
	}
	return 0
}

func (wm *WaitMap) Len() int {
	return len(wm.pool)
}
func (wm *WaitMap) Print() {
	buf := bytes.NewBuffer([]byte{})
	buf.WriteString("等待统计-----------------")
	wm.RLock()
	for k, v := range wm.pool {
		fmt.Fprintln(buf, k.Pretty(), v)
	}
	wm.RUnlock()
	buf.WriteString("等待统计结束-----------------")
	log.Println(buf.String())
}

type Stat struct {
	Success int64
	Error   int64
	CtxDone int64
	All     int64
	Wait    WaitMap
}

func (s *Stat) Reset() {
	atomic.StoreInt64(&s.Success, 0)
	atomic.StoreInt64(&s.Error, 0)
	atomic.StoreInt64(&s.CtxDone, 0)
	atomic.StoreInt64(&s.All, 0)
}

func (s *Stat) Get() (int, int64, int64, int64, int64) {
	return s.Wait.Len(), atomic.LoadInt64(&s.Success), atomic.LoadInt64(&s.Error), atomic.LoadInt64(&s.CtxDone), atomic.LoadInt64(&s.All)
}

func (s *Stat) Add(ss int64, e int64, c int64, a int64) {
	if ss != 0 {
		atomic.AddInt64(&s.Success, ss)
	}
	if e != 0 {
		atomic.AddInt64(&s.Error, e)
	}
	if c != 0 {
		atomic.AddInt64(&s.CtxDone, c)
	}
	if a != 0 {
		atomic.AddInt64(&s.All, a)
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
			v1, v2, v3, v4, v5 := Default.Get()
			sum := atomic.LoadInt64(&Default.Wait.Sum)
			log.Printf("并发 %d,成功 %d,失败 %d, 超时 %d, 总数 %d, 所有 %d\n", v1, v2, v3, v4, sum, v5)
			Default.Wait.Print()
			Default.Reset()
		}
	}()
}
