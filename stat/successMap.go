package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
	"time"
)

type SuccessMap struct {
	sm map[peer.ID]*record
	sync.RWMutex
}

type record struct {
	num        uint64
	lastModify time.Time
}

func (r *record) Add(num uint64) {
	if time.Now().Sub(r.lastModify) > time.Second*10 {
		r.num = 0
	}

	r.num += num
	r.lastModify = time.Now()
}

func (r *record) GetNum() uint64 {
	t := time.Second*10 - time.Now().Sub(r.lastModify)
	if t < 0 {
		t = time.Second
	}
	t = t / time.Second
	return uint64(t)*10 + r.num*5
}

func (sm *SuccessMap) Add(id peer.ID) {
	sm.Lock()
	defer sm.Unlock()

	if v, ok := sm.sm[id]; ok {
		v.Add(1)
	} else {
		sm.sm[id] = &record{1, time.Now()}
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
		successNum, ok := sm.sm[id]
		if ok {
			item = &s{id, successNum.GetNum()}
		} else {
			item = &s{id, 150}
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
	return SuccessMap{make(map[peer.ID]*record, 0), sync.RWMutex{}}
}
