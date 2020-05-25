package client

import (
	"sync"
	"time"
)

type SpeedCounter struct {
	d   []time.Duration
	cap int
	sync.RWMutex
}

func (sc *SpeedCounter) Push(duration time.Duration) {
	sc.Lock()
	defer sc.Unlock()

	if len(sc.d) >= sc.cap {
		sc.d = append(sc.d[1:], duration)
	} else {
		sc.d = append(sc.d, duration)
	}
}

func (sc *SpeedCounter) AvgSpeed() time.Duration {
	sc.RLock()
	defer sc.RUnlock()

	var sum time.Duration
	for _, v := range sc.d {
		sum += v
	}
	if l := len(sc.d); l != 0 {
		return sum / time.Duration(l)
	} else {
		return 0
	}
}

func NewSpeedCounter(cap int) *SpeedCounter {
	return &SpeedCounter{
		d:   make([]time.Duration, cap),
		cap: cap,
	}
}
