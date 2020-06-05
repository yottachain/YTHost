package client

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"log"
	"sync"
	"time"
)

type delay struct {
	d          time.Duration
	count      int64
	modifyTime time.Time
}

func (dly delay) GetNum() time.Duration {
	if time.Now().Sub(dly.modifyTime) < time.Second*2 {
		return dly.d
	}

	dly.d = dly.d / time.Duration(dly.count)
	dly.count = 1
	dly.modifyTime = time.Now()

	return dly.d
}

func (dly delay) Add(d time.Duration) {
	dly.d += d
	dly.count++

	if time.Now().Sub(dly.modifyTime) < time.Second*2 {
		return
	}

	dly.d = dly.d / time.Duration(dly.count)
	dly.count = 1
	dly.modifyTime = time.Now()
}

func (dly delay) Print(id string) {
	log.Printf("延迟 [%s] used %d ms, count %d \n", id, dly.d.Milliseconds(), dly.count)
}

func newDelay(d time.Duration) delay {
	return delay{d, 1, time.Now()}
}

type SpeedCounter struct {
	d  delay
	id string
	sync.RWMutex
}

func (sc *SpeedCounter) Push(duration time.Duration) {
	sc.Lock()
	defer sc.Unlock()

	sc.d.Add(duration)
}

func (sc *SpeedCounter) AvgSpeed() time.Duration {
	sc.RLock()
	defer sc.RUnlock()

	sc.d.Print(sc.id)
	return sc.d.GetNum()
}

func NewSpeedCounter(id peer.ID) *SpeedCounter {
	return &SpeedCounter{
		d:  newDelay(0),
		id: id.Pretty(),
	}
}
