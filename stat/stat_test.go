package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"testing"
	"time"
)

func TestOutPut(t *testing.T) {
	id := peer.ID(string(11))
	DefaultStatTable.Put(id, &ClientStat{})
	for i := 0; i < 30; i++ {
		<-time.After(time.Second)
		s, _ := DefaultStatTable.GetOrPut(id, &ClientStat{})
		s.Success++
	}
}
