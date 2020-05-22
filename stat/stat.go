// stat
// 统计模块
//
package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type ClientStat struct {
	Wait               uint64
	Success            uint64
	Error              uint64
	CtxDone            uint64
	Refuse             uint64
	PreRequestTime     time.Time
	RequestHandleSpeed uint64
	Outtime            time.Duration
	sync.RWMutex
}

func (cs *ClientStat) Set(setFunc func(cs *ClientStat)) {
	cs.Lock()
	defer cs.Unlock()

	setFunc(cs)
}

func (cs *ClientStat) Print(id string) {
	cs.RLock()
	defer cs.RUnlock()

	log.Printf("[ythost stat] id %s waite %d success %d error %d ctx timeout %d speed %d interval %d ms \n",
		id,
		cs.Wait,
		cs.Success,
		cs.Error,
		cs.CtxDone,
		cs.RequestHandleSpeed,
		cs.Outtime.Milliseconds(),
	)
}

type StatTable struct {
	table map[peer.ID]*ClientStat
	sync.RWMutex
}

var DefaultStatTable = StatTable{
	table: make(map[peer.ID]*ClientStat),
}

func (st *StatTable) List() []peer.ID {
	st.RLock()
	defer st.RUnlock()

	var res = make([]peer.ID, len(st.table))

	i := 0
	for k, _ := range st.table {
		res[i] = k
		i++
	}

	return res
}

func (st *StatTable) GetRow(key peer.ID) *ClientStat {
	st.RLock()
	defer st.RUnlock()

	stat, ok := st.table[key]
	if !ok {
		return nil
	} else {
		return stat
	}
}

func (st *StatTable) Put(key peer.ID, stat *ClientStat) {
	st.Lock()
	defer st.Unlock()

	st.table[key] = stat
}

func (st *StatTable) GetOrPut(key peer.ID, stat *ClientStat) (*ClientStat, bool) {
	ok := false

	_stat := st.GetRow(key)
	if _stat != nil {
		ok = true
	} else {
		_stat = stat
		st.Put(key, _stat)
	}

	return _stat, ok
}

func OutPut(fl io.Writer) {
}

func init() {
	fl, err := os.OpenFile("ythost.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(fl)
	}
}
