package ClientManage

import (
	"encoding/json"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type AddrsBook struct {
	book map[peer.ID][]multiaddr.Multiaddr
	sync.RWMutex
}

func (ab *AddrsBook) Add(id peer.ID, addrs []multiaddr.Multiaddr) {
	ab.Lock()
	defer ab.Unlock()

	ab.book[id] = addrs
}
func (ab *AddrsBook) Remove(id peer.ID) {
	ab.Lock()
	defer ab.Unlock()

	delete(ab.book, id)
}

func (ab *AddrsBook) Get(id peer.ID) (res []multiaddr.Multiaddr, ok bool) {
	ab.RLock()
	defer ab.RUnlock()
	res, ok = ab.book[id]
	return
}

func (ab *AddrsBook) List() map[peer.ID][]multiaddr.Multiaddr {
	ab.RLock()
	defer ab.RUnlock()

	return ab.book
}

// url = http://39.105.184.162:8082/active_nodes
func NewAddBookFromServer(url string) (addrsBook *AddrsBook, err error) {
	type addrInfo struct {
		Nodeid string `json:"nodeid"`
		IP     string `json:"ip"`
	}

	resp, err := http.Get(url)
	if err != nil {
		return
	}

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var addrs []addrInfo

	err = json.Unmarshal(buf, &addrs)
	if err != nil {
		return
	}

	addrsBook = NewAddrsBook()
	for _, v := range addrs {
		id, err2 := peer.Decode(v.Nodeid)
		if err2 != nil {
			continue
		}
		addr, err2 := multiaddr.NewMultiaddr(v.IP)
		if err2 != nil {
			continue
		}

		addrsBook.Add(id, []multiaddr.Multiaddr{addr})
	}

	return
}

func NewAddrsBook() *AddrsBook {
	return &AddrsBook{book: make(map[peer.ID][]multiaddr.Multiaddr)}
}

func (mng *Manager) Keep(d time.Duration) {
	for {
		<-time.After(d)
		for k, v := range mng.AB.List() {
			mng.Connect(k, v)
		}
	}
}
