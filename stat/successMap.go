package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
)

type SuccessMap struct {
	pool map[peer.ID]uint64
	sync.RWMutex
}

func (sm *SuccessMap) Add(id peer.ID) {
	sm.Lock()
	defer sm.Unlock()

	if v, ok := sm.pool[id]; ok {
		sm.pool[id] = v + 1
	} else {
		sm.pool[id] = 1
	}
}

func (sm *SuccessMap) SortList(ids ...peer.ID) []peer.ID {
	sm.RLock()
	defer sm.RUnlock()

	type s struct {
		ID      peer.ID
		Success uint64
	}

	var list = make([]*s, 0)
	for _, id := range ids {
		var item *s
		successNum, ok := sm.pool[id]
		if ok {
			item = &s{id, successNum}
		} else {
			item = &s{id, 10}
		}

		for k2, v2 := range list {
			if v2.Success < item.Success {
				list[k2], item = item, list[k2]
			}
		}

		list = append(list, item)
	}

	var res []peer.ID = make([]peer.ID, len(list))

	for k, v := range list {
		res[k] = v.ID
	}

	return res
}

func NewSuccessMap() SuccessMap {
	return SuccessMap{make(map[peer.ID]uint64, 0), sync.RWMutex{}}
}
