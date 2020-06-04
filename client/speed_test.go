package client

import (
	"testing"
	"time"
)

func TestSpeedDelay(t *testing.T) {
	d := newDelay(0)

	for i := 0; i < 10; i++ {
		d.Add(time.Second * time.Duration(i))
	}
}
