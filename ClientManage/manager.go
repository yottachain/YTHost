package ClientManage

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
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

func (mng *Manager) Get(id peer.ID) (*client.YTHostClient, error) {
	clt, ok := mng.store.GetClient(id)
	if ok {
		return clt, nil
	} else {
		if mas, ok := mng.AB.Get(id); ok {
			go mng.Connect(id, mas)
		}
		return nil, fmt.Errorf("node not available")
	}
}

func (mng *Manager) GetOrConnect(id peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*5)
	return mng.store.Get(ctx, id, mas)
}

//func (mng *Manager) Keep(d time.Duration) {
//	for {
//		<-time.After(d)
//	}
//}
//
//func (mng *Manager) Ping(id peer.ID) (*client.YTHostClient, error) {
//	mng.Get()
//}
