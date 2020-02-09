package service

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTCrypto"
	ci "github.com/yottachain/YTHost/clientInterface"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/encrypt"
	"github.com/yottachain/YTHost/peerInfo"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

var (
	Trace   *log.Logger // 记录所有日志
	Info    *log.Logger // 重要的信息
	Warning *log.Logger // 需要注意的信息
	Error   *log.Logger // 非常严重的问题
)

func init() {
	file, err := os.OpenFile("errors.txt",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error log file:", err)
	}

	Trace = log.New(ioutil.Discard,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(os.Stdout,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(os.Stdout,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(io.MultiWriter(file, os.Stderr),
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

type MsgId int32

type Handler func(requestData []byte, head Head) ([]byte, error)

type HandlerMap map[int32]Handler

type Head struct {
	MsgId        int32
	RemotePeerID peer.ID
	RemoteAddrs  []multiaddr.Multiaddr
	RemotePubKey []byte
}

// RegisterHandler 注册消息处理器
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

// RegisterGlobalMsgHandler 注册全局消息处理器
func (hm HandlerMap) RegisterGlobalMsgHandler(handlerFunc Handler) {
	hm.registerHandler(0x0, handlerFunc)
}

// RemoveHandler 移除消息处理器
func (hm HandlerMap) RemoveHandler(id int32) {
	delete(hm, id)
}
func (hm HandlerMap) RemoveGlobalHandler() {
	delete(hm, 0x0)
}


type MsgService struct {
	Handler HandlerMap
	Pi      peerInfo.PeerInfo
	LocalPriKey crypto.PrivKey
	msgPriMap *sync.Map
	ClientStore *clientStore.ClientStore
	LocalPeerID peer.ID
}

type Request struct {
	MsgId          int32
	ReqData        []byte
	RemotePeerInfo PeerInfo
	//目标的peer ID 对中继来说 如果和自己的ID不匹配则转发
	DstID			peer.ID
}

type Response struct {
	Data []byte
}

type IdToMsgPriKey struct {
	ID     peer.ID
	MsgPriKey		[]byte
}

func (ms *MsgService) Ping(req string, res *string) error {
	*res = "pong"
	return nil
}

func (ms *MsgService) RegisterMsgPriKey(clinetMsgKey IdToMsgPriKey, res *bool) error {
	ms.msgPriMapinit()

	hostPriKey, _ := ms.LocalPriKey.Raw()

	peerId := clinetMsgKey.ID
	msgPriKey := clinetMsgKey.MsgPriKey

	pr := append([]byte{0x80}, hostPriKey[0:32]...)

	finalMsgPriKey, err:= YTCrypto.ECCDecrypt(msgPriKey, base58.Encode(pr))
	if err != nil {
		*res = false
		return err
	}

	//fmt.Printf("server recive client gen prikey is %s\n", base58.Encode(finalMsgPriKey))

	ms.msgPriMap.Store(peerId, finalMsgPriKey)
	*res = true

	return nil
}

func (ms *MsgService) HandleMsg(req Request, data *Response) error {
	ms.msgPriMapinit()

	if ms.Handler == nil {
		Error.Printf("no handler %d\n", req.MsgId)
		return fmt.Errorf("no handler %x", req.MsgId)
	}

	// 0x0～0x10 为保留全局消息处理器
	h, ok := ms.Handler[0x0]
	// 如果没有全局处理器，调用局部处理器
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

	//解密消息
	_k, ok := ms.msgPriMap.Load(head.RemotePeerID)
	if !ok {
		Error.Printf("no msgPriKey %s\n", head.RemotePeerID.String())
		return fmt.Errorf("no msgPriKey %s", head.RemotePeerID.String())
	}
	msgKey := _k.([]byte)

	//fmt.Printf("before at aes decode msg is %s\n", string(req.ReqData))

	reqData, err := encrypt.AesCBCDncrypt(req.ReqData, msgKey)
	if err != nil {
		Error.Printf("AesCBCDncrypt msg error %s\n", head.RemotePeerID.String())
		return fmt.Errorf("AesCBCDncrypt msg error %x", head.RemotePeerID)
	}

	//fmt.Printf("secret key [%s] ---------> after at aes decode msg is [%s]\n", base58.Encode(msgKey), string(reqData))

	if ok {
		//目标ID不是自己的ID就转发出去
		dstr := req.DstID.String()
		lstr := ms.LocalPeerID.String()
		if dstr != lstr {
			resdata, err := ms.transpondMsg(req.DstID, req.MsgId, reqData)
			if nil != err {
				Error.Printf("peerid:%s  transpondMsg ID: %s---%s\n",head.RemotePeerID.String(), ms.LocalPeerID.String(), err)
				return err
			}
			//Error.Printf("peerid:%s  transpondMsg ID: %s\n", head.RemotePeerID.String(), ms.LocalPeerID.String())
			aesData, err := encrypt.AesCBCEncrypt(resdata, msgKey)
			if err != nil {
				Error.Printf("transpondMsg peerid:%s respones AesCBCEncrypt msg error %s\n", head.RemotePeerID.String(), err)
				return fmt.Errorf("respones AesCBCEncrypt msg error %x", err)
			}
			data.Data = aesData
			return nil
		}

		if resdata, err := h(reqData, head); err != nil {
			return nil
		} else {
			aesData, err := encrypt.AesCBCEncrypt(resdata, msgKey)
			if err != nil {
				Error.Printf("peerid:%s respones AesCBCEncrypt msg error %s\n",head.RemotePeerID.String(),  err)
				return fmt.Errorf("respones AesCBCEncrypt msg error %x", err)
			}
			data.Data = aesData
			return nil
		}
	} else {
		Error.Printf("no handler %d\n", req.MsgId)
		return fmt.Errorf("no handler %x", req.MsgId)
	}
}

func (ms *MsgService) msgPriMapinit() {
	if ms.msgPriMap == nil {
		ms.msgPriMap = &sync.Map{}
	}
}

//func (ms *MsgService) transpondMsg() ([]byte, error){
func (ms *MsgService) transpondMsg(DstID peer.ID, msgId int32, msg []byte) ([]byte, error) {
	_c, ok := ms.ClientStore.Load(DstID)

	if !ok {
		return nil, fmt.Errorf("relay not connect with peer ID: [%s]\n", DstID.String())
	}
	c := _c.(ci.YTHClient)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()
	retData, err := c.SendMsg(ctx, DstID, msgId, msg)
	if nil != err {
		return nil, fmt.Errorf("relay send msg to peer ID: [%s] fail\n", DstID.String())
	}
	return  retData, nil
}
