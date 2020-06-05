package ClientManage

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/stat"
	"time"
)

type Manager struct {
	AB    *AddrsBook
	store *clientStore.ClientStore
}

func NewManager(store *clientStore.ClientStore) (*Manager, error) {
	ab, err := NewAddBookFromServer("http://39.105.184.162:8082/active_nodes")
	if err != nil {
		return nil, err
	}

	var mng = Manager{AB: ab, store: store}

	for k, v := range mng.AB.List() {
		go mng.Connect(k, v)
	}

	return &mng, err
}

func (mng *Manager) Connect(id peer.ID, mas []multiaddr.Multiaddr) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	mng.store.Get(ctx, id, mas)
}

func (mng *Manager) Get(id peer.ID, addrs []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	if _, ok := mng.AB.Get(id); !ok {
		mng.AB.Add(id, addrs)
		ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
		return mng.store.Get(ctx, id, addrs)
	}
	clt, ok := mng.store.GetClient(id)
	if ok {
		return clt, nil
	} else {
		if _, ok := mng.AB.Get(id); ok {
			go mng.Connect(id, addrs)
		}
		return nil, fmt.Errorf("node not available")
	}
}

func (mng *Manager) GetOrConnect(id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	return mng.store.Get(ctx, id, mas)
}

func (mng *Manager) Keep(d time.Duration) {
	for {
		<-time.After(d)
		for k, v := range mng.AB.List() {
			go mng.Connect(k, v)
		}
	}
}

func (mng *Manager) GetOptNodes(ids []string, optNum int) []string {
	type score struct {
		ID       peer.ID
		Duration time.Duration
	}

	var list = make([]*score, 0)
	for _, v := range ids {
		id, err := peer.Decode(v)
		if err != nil {
			continue
		}
		list = append(list, &score{id, stat.DefaultSpeedCounter.Get(id)})
	}

	for k, _ := range list {
		for i := k; i < len(list); i++ {
			if list[k].Duration > list[i].Duration {
				list[k], list[i] = list[i], list[k]
			}
		}
	}

	var l = len(list)
	if l > optNum {
		l = optNum
	}

	var res = make([]string, l)

	for i := 0; i < l; i++ {
		res[i] = list[i].ID.Pretty()
	}

	return res
}

//func PA2ids(pas ...peer.AddrInfo) []string {
//	res := make([]string, len(pas))
//	for k, v := range pas {
//		res[k] = v.ID.Pretty()
//	}
//	return res
//}
