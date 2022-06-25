package service

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTHost/peerInfo"
)

type MsgId int32

type Handler func(requestData []byte, head Head) ([]byte, error)

type HandlerMap map[int32]Handler

type Head struct {
	MsgId        int32
	RemotePeerID peer.ID
	RemoteAddrs  []multiaddr.Multiaddr
	RemotePubKey []byte
}

func (hm HandlerMap) RegisterHandler(id int32, handlerFunc Handler) error {
	if id < 0x10 {
		return fmt.Errorf("msgID need >= 0x10")
	}
	hm.registerHandler(id, handlerFunc)
	return nil
}

func (hm HandlerMap) registerHandler(id int32, handlerFunc Handler) {
	if hm == nil {
		hm = make(HandlerMap)
	}
	hm[id] = handlerFunc
}

func (hm HandlerMap) RegisterGlobalMsgHandler(handlerFunc Handler) {
	hm.registerHandler(0x0, handlerFunc)
}

func (hm HandlerMap) RemoveHandler(id int32) {
	delete(hm, id)
}
func (hm HandlerMap) RemoveGlobalHandler() {
	delete(hm, 0x0)
}

type MsgService struct {
	Handler HandlerMap
	Pi      peerInfo.PeerInfo
}

type Request struct {
	MsgId          int32
	ReqData        []byte
	RemotePeerInfo PeerInfo
}

type Response struct {
	Data       []byte
	ReturnTime time.Time
}

func (ms *MsgService) Ping(req string, res *string) error {
	*res = "pong"
	return nil
}

func (ms *MsgService) HandleMsg(req Request, data *Response) error {
	if ms.Handler == nil {
		return fmt.Errorf("no handler %x", req.MsgId)
	}
	h, ok := ms.Handler[0x0]
	if !ok {
		h, ok = ms.Handler[req.MsgId]
	}
	head := Head{}
	head.MsgId = req.MsgId
	head.RemotePeerID = req.RemotePeerInfo.ID
	head.RemotePubKey = req.RemotePeerInfo.PubKey
	for _, v := range req.RemotePeerInfo.Addrs {
		ma, _ := multiaddr.NewMultiaddr(v)
		head.RemoteAddrs = append(head.RemoteAddrs, ma)
	}
	if ok {
		if resdata, err := h(req.ReqData, head); err != nil {
			return err
		} else {
			data.Data = resdata
			data.ReturnTime = time.Now()
			return nil
		}
	} else {
		return fmt.Errorf("no handler %x", req.MsgId)
	}
}
