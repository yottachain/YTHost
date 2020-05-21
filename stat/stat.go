// stat
// 统计模块
//
package stat

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"os"
	"sync"
	"time"
)

type ClientStat struct {
	Wait              uint64
	Success           uint64
	Error             uint64
	Refuse            uint64
	RequestHandleTime time.Duration
	Outtime           time.Duration
	sync.RWMutex
}

type TotalStat struct {
	Success uint64
	Error   uint64
	Current uint64
	sync.RWMutex
}

func (cs *ClientStat) Set(setFunc func(cs *ClientStat)) {
	cs.Lock()
	defer cs.Unlock()

	setFunc(cs)
}

type StatTable struct {
	table map[peer.ID]*ClientStat
	total *TotalStat
	sync.RWMutex
}

var DefaultStatTable = StatTable{
	table:   make(map[peer.ID]*ClientStat),
	total:   &TotalStat{},
	RWMutex: sync.RWMutex{},
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

func (st *StatTable) Total() *TotalStat {
	return st.total
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

func OutPut() {
	fl, err := os.OpenFile("ythost_stat.log", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer fl.Close()

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

	DefaultStatTable.total.Lock()
	fmt.Fprintf(fl, "speed success %d c/s,error %d c/s ,concurrent %d\n", DefaultStatTable.total.Success/5, DefaultStatTable.total.Error/5, DefaultStatTable.total.Current)
	DefaultStatTable.total.Success = 0
	DefaultStatTable.total.Error = 0
	DefaultStatTable.total.Unlock()
}

func init() {
	go func() {
		for {
			OutPut()
			<-time.After(time.Second * 5)
		}
	}()
}
