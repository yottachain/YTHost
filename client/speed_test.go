package client

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestSpeedDelay(t *testing.T) {
	d := newDelay(0)

	go func() {
		for i := 0; i < 10000; i++ {
			<-time.After(time.Millisecond * time.Duration(rand.Intn(500)))
			d.Add(time.Second * time.Duration(rand.Intn(10)))
		}
	}()

	for {
		<-time.After(time.Second)
		fmt.Println(d.GetNum())
		d.Print("11")
	}

}
