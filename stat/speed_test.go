package stat

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"math/rand"
	"testing"
	"time"
)

func TestSpeedDelay(t *testing.T) {
	d := SpeedCounter{}

	go func() {
		for i := 0; i < 10000; i++ {
			<-time.After(time.Millisecond * time.Duration(rand.Intn(500)))
			d.Push(peer.ID("111"), time.Second*time.Duration(rand.Intn(10)))
		}
	}()

	for {
		<-time.After(time.Millisecond)
		fmt.Println(d.Get(peer.ID("111")))
	}

}
