package stat

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
	"testing"
)

func TestSuccessMap_SortList(t *testing.T) {
	var m SuccessMap = SuccessMap{make(map[peer.ID]uint64, 0), sync.RWMutex{}}
	id1, _ := peer.Decode("16Uiu2HAmViSBShkSuMjuRr7XvfKYhSKxPxvbANXrTLNmjD5isFpz")
	m.Add(id1)
	m.Add(id1)
	m.Add(id1)
	m.Add(id1)
	m.Add(id1)
	m.Add(id1)

	m.Add(id1)
	m.Add(id1)
	m.Add(id1)

	m.Add(id1)
	m.Add(id1)
	m.Add(id1)

	for k, v := range m.SortList(id1, "asdasdasd") {
		fmt.Println(k, v.Pretty())
	}
}
