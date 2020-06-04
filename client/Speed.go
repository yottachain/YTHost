package client

import (
	"sync"
	"time"
)

type delay struct {
	d          time.Duration
	modifyTime time.Time
}

func (dly delay) GetNum() time.Duration {
	t := (time.Second * 20) - time.Now().Sub(dly.modifyTime)
	if t < 0 {
		t = 0
	}
	return t / time.Second * dly.d
}

func newDelay(d time.Duration) delay {
	return delay{d, time.Now()}
}

type SpeedCounter struct {
	d   []delay
	cap int
	sync.RWMutex
}

func (sc *SpeedCounter) Push(duration time.Duration) {
	sc.Lock()
	defer sc.Unlock()

	if len(sc.d) >= sc.cap {
		sc.d = append(sc.d[1:], newDelay(duration))
	} else {
		sc.d = append(sc.d, newDelay(duration))
	}
}

func (sc *SpeedCounter) AvgSpeed() time.Duration {
	sc.RLock()
	defer sc.RUnlock()

	var sum time.Duration
	for _, v := range sc.d {
		sum += v.GetNum()
	}
	if l := len(sc.d); l != 0 {
		return sum
	} else {
		return 0
	}
}

func NewSpeedCounter(cap int) *SpeedCounter {
	return &SpeedCounter{
		d:   make([]delay, cap),
		cap: cap,
	}
}
