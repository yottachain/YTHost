package ClientManage

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/stat"
	"math/rand"
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

func (mng *Manager) GetOptNodes(optNum int, randNum int) []peer.AddrInfo {
	cs := mng.store

	type Source struct {
		ID       peer.ID
		Addrs    []multiaddr.Multiaddr
		Duration time.Duration
	}
	var list []Source = make([]Source, len(mng.AB.List()))
	var res = make([]peer.AddrInfo, optNum+randNum)

	var i = 0
	for k, v := range mng.AB.List() {

		var current Source
		current.ID = k
		current.Addrs = v

		if ac, ok := cs.Map.Load(k); ok {
			client := ac.(*client.YTHostClient)

			wait := stat.Default.Wait.Get(k)
			current.Duration = client.Sc.AvgSpeed() * (time.Duration(wait) + 1)
		}

		list[i] = current
		i++
	}

	for i := 0; i < len(list); i++ {
		for j := i; j < len(list); j++ {
			if list[j].Duration < list[i].Duration {
				temp := list[i]
				list[i] = list[j]
				list[j] = temp
			}
		}
	}

	for k, v := range list[:optNum] {
		res[k].ID = v.ID
		res[k].Addrs = v.Addrs
	}

	for i := 0; i < randNum; i++ {
		k := rand.Intn(randNum) + optNum
		res[i+optNum].ID = list[k].ID
		res[i+optNum].Addrs = list[k].Addrs

		// 随机交换
		j := rand.Intn(randNum + optNum - 1)
		res[i+optNum], res[j] = res[j], res[i+optNum]
	}

	return res
}

func PA2ids(pas ...peer.AddrInfo) []string {
	res := make([]string, len(pas))
	for k, v := range pas {
		res[k] = v.ID.Pretty()
	}
	return res
}
