package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/exp/rand"
	"testing"
	"time"
)

func TestOutPut(t *testing.T) {
	for i := 0; i < 30; i++ {
		<-time.After(time.Second)
		DefaultStatTable.Put(peer.ID(string(i)), &ClientStat{Wait: rand.Uint64(), Success: rand.Uint64(), RequestHandleTime: time.Duration(rand.Int63())})
	}
}
