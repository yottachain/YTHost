package stat

import "sync"

type ConnStat struct {
	sync.Mutex
	CliConnCount uint64
	SerConnCount uint64
	SerCloseChanl chan struct{}
}

func NewCs () (cs *ConnStat) {
	cs = &ConnStat{
		Mutex:         sync.Mutex{},
		CliConnCount:  0,
		SerConnCount:  0,
		SerCloseChanl: make(chan struct{}, 1),
	}

	go cs.ListenSerClose()

	return
}

func (cs *ConnStat) CccAdd () {
	cs.Lock()
	defer cs.Unlock()
	cs.CliConnCount++
}

func (cs *ConnStat) CccSub () {
	cs.Lock()
	defer cs.Unlock()
	cs.CliConnCount--
}

func (cs *ConnStat) GetCliconnCount() uint64 {
	cs.Lock()
	defer cs.Unlock()
	return cs.CliConnCount
}

func (cs *ConnStat) SccAdd () {
	cs.Lock()
	defer cs.Unlock()
	cs.SerConnCount++
}

func (cs *ConnStat) SccSub () {
	cs.Lock()
	defer cs.Unlock()
	cs.SerConnCount--
}

func (cs *ConnStat) GetSerconnCount() uint64 {
	cs.Lock()
	defer cs.Unlock()
	return cs.SerConnCount
}

func (cs *ConnStat) ListenSerClose() {
	for {
		<-cs.SerCloseChanl
		cs.SccSub()
	}
}
