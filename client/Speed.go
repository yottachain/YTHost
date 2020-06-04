package client

import (
	"sync"
	"time"
)

type delay struct {
	d          time.Duration
	count      int64
	modifyTime time.Time
}

func (dly delay) GetNum() time.Duration {
	if time.Now().Sub(dly.modifyTime) < time.Millisecond*2 {
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

	if time.Now().Sub(dly.modifyTime) < time.Millisecond*2 {
		return
	}
	dly.d = dly.d / time.Duration(dly.count)
	dly.count = 1
	dly.modifyTime = time.Now()
}

func newDelay(d time.Duration) delay {
	return delay{d, 0, time.Now()}
}

type SpeedCounter struct {
	d   delay
	cap int
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

	return sc.d.GetNum()
}

func NewSpeedCounter(cap int) *SpeedCounter {
	return &SpeedCounter{
		d:   newDelay(0),
		cap: cap,
	}
}
