// stat
// 统计模块
//
package stat

import (
	"github.com/libp2p/go-libp2p-core/peer"
	"io"
	"sync"
	"time"
)

type ClientStat struct {
	Wait               uint64
	Success            uint64
	Error              uint64
	Refuse             uint64
	PreRequestTime     time.Time
	PreSuccess         uint64
	RequestHandleSpeed uint64
	Outtime            time.Duration
	sync.RWMutex
}

func (cs *ClientStat) Set(setFunc func(cs *ClientStat)) {
	cs.Lock()
	defer cs.Unlock()

	setFunc(cs)
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
	//ids := DefaultStatTable.List()
	//
	//fmt.Fprintf(fl, "index,id,wait,success,error,requestTime,outtime,refuse\n")
	//for i, id := range ids {
	//	row := DefaultStatTable.GetRow(id)
	//	if row != nil {
	//		row.RLock()
	//		fmt.Fprintf(
	//			fl,
	//			"%d,%s,%d,%d,%d,%d ms,%d ms,%d\n",
	//			i,
	//			id.Pretty(),
	//			row.Wait,
	//			row.Success,
	//			row.Error,
	//			row.RequestHandleTime.Milliseconds(),
	//			row.Outtime.Milliseconds(),
	//			row.Refuse,
	//		)
	//		row.RUnlock()
	//	}
	//}
}
