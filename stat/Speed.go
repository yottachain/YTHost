package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
	"time"
)

type delay struct {
	id         peer.ID
	d          time.Duration
	count      int64
	modifyTime time.Time
	sync.Mutex
}

func (dly *delay) GetNum() time.Duration {
	dly.Lock()
	defer dly.Unlock()

	//buf := bytes.NewBuffer([]byte{})
	//
	//fmt.Fprintf(buf, "延迟 [%s]", dly.id)
	//fmt.Fprintf(buf, "used: %dms ", dly.d.Milliseconds())
	//fmt.Fprintf(buf, "count: %d ", dly.count)
	//fmt.Fprintf(buf, "lasttime: %v ", dly.modifyTime)
	//fmt.Fprintf(buf, "next:: ")
	//
	if (time.Now().Sub(dly.modifyTime) > time.Second*10) && dly.count == 1 {
		dly.d = dly.d - dly.d/10
		dly.modifyTime = time.Now()
	} else if dly.count > 1 {
		dly.d = dly.d / time.Duration(dly.count)
		dly.count = 1
		dly.modifyTime = time.Now()
	}

	d := dly.d
	//
	//fmt.Fprintf(buf, "used: %dms ", dly.d.Milliseconds())
	//fmt.Fprintf(buf, "count: %d ", dly.count)
	//fmt.Fprintf(buf, "lasttime: %v ", dly.modifyTime)
	//
	//go log.Println(buf.String())
	return d
}

func (dly *delay) Add(d time.Duration) {
	dly.Lock()
	defer dly.Unlock()

	dly.d += d
	dly.count++
}

func newDelay(id peer.ID, d time.Duration) *delay {
	return &delay{id, d, 1, time.Now(), sync.Mutex{}}
}

type SpeedCounter struct {
	dmap sync.Map
}

func (sc *SpeedCounter) Push(id peer.ID, d time.Duration) {
	if ac, ok := sc.dmap.LoadOrStore(id, newDelay(id, d)); ok {
		dly := ac.(*delay)
		dly.Add(d)
	}
}

func (sc *SpeedCounter) Get(id peer.ID) time.Duration {
	if ac, ok := sc.dmap.Load(id); ok {
		dly := ac.(*delay)

		return dly.GetNum()
	} else {
		return 0
	}
}

var DefaultSpeedCounter = SpeedCounter{sync.Map{}}
