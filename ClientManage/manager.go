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

func ids2pids(ids []string) []peer.ID {
	pids := make([]peer.ID, len(ids))
	for k, v := range ids {
		pids[k], _ = peer.Decode(v)
	}
	return pids
}

func (mng *Manager) GetOptNodes(ids []string, optNum int, randNum int) []peer.ID {

	pids := ids2pids(ids)

	list := stat.Default.SMap.SortList(pids...)
	if optNum+randNum > len(list) {
		fmt.Println("选取节点超过当前节点数")
		return nil
	}

	var res = make([]peer.ID, optNum+randNum)

	for i := 0; i < optNum; i++ {
		res[i] = list[i]
	}

	// 添加随机节点
	for i := 0; i < randNum; i++ {
		item := list[rand.Intn(len(list)-optNum)+optNum]
		res[i+optNum] = item

		// 随机交换位置
		index := rand.Intn(optNum - 1)
		res[i+optNum], res[index] = res[index], res[i+optNum]
	}

	return res
}

func PA2ids(pas ...peer.ID) []string {
	res := make([]string, len(pas))
	for k, v := range pas {
		res[k] = v.Pretty()
	}
	return res
}
