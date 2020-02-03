package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/yottachain/YTCrypto"
	ci "github.com/yottachain/YTHost/clientInterface"
	"github.com/yottachain/YTHost/encrypt"
	"github.com/yottachain/YTHost/service"
	"golang.org/x/crypto/ripemd160"
	"net/rpc"
	//"github.com/yottachain/YTHost/rpc"
)

type YTHostClient struct {
	*rpc.Client
	localPeerID     peer.ID
	localPeerAddrs  []string
	localPeerPubKey []byte
	isClosed        bool
	msgPriKey		[]byte
	RemotePeerID	peer.ID			//链接到的目标ID
	DstID			peer.ID			//发送到的目标ID	如果不通过中继 RemotePeerID,DstID 相同 否则不同
}

func (yc *YTHostClient) RemotePeer() (peer.AddrInfo, error) {
	var pi service.PeerInfo
	var ai peer.AddrInfo

	if err := yc.Call("as.RemotePeerInfo", "", &pi); err != nil {
		return ai, err
	}
	ai.ID = pi.ID
	for _, addr := range pi.Addrs {
		ma, _ := multiaddr.NewMultiaddr(addr)
		ai.Addrs = append(ai.Addrs, ma)
	}

	return ai, nil
}

func (yc *YTHostClient) RemotePeerPubkey() (crypto.PubKey, error) {
	var pi service.PeerInfo
	var pk crypto.PubKey
	if err := yc.Call("as.RemotePeerInfo", "", &pi); err != nil {
		fmt.Println(err)
		return pk, err
	}
	pk, err := crypto.UnmarshalPublicKey(pi.PubKey)
	if err != nil {
		return pk, fmt.Errorf("get remote pubkey fail\n")
	}
	return pk, nil
}

func (yc *YTHostClient) LocalPeer() peer.AddrInfo {
	pi := peer.AddrInfo{}
	pi.ID = yc.localPeerID
	for _, v := range yc.localPeerAddrs {
		ma, _ := multiaddr.NewMultiaddr(v)
		pi.Addrs = append(pi.Addrs, ma)
	}
	return pi
}

func (yc *YTHostClient) SendMsgPriKey() (error) {
	pk, err := yc.RemotePeerPubkey()
	if err != nil {
		return err
	}

	pkbyte, err := pk.Raw()
	if err != nil {
		return err
	}

	msgkey := yc.msgPriKey

	//fmt.Printf("client gen prikey is %s\n", base58.Encode(msgkey))

	hasher := ripemd160.New()
	hasher.Write(pkbyte)
	sum := hasher.Sum(nil)
	pkbyte = append(pkbyte, sum[0:4]...)

	pkstr := base58.Encode(pkbyte)

	//fmt.Printf("peer publickey is %s\n", pkstr)

	//加密
	cipherKkey, err := YTCrypto.ECCEncrypt(msgkey, pkstr)
	if err != nil {
		return err
	}

	args := service.IdToMsgPriKey{
		ID: yc.localPeerID,
		MsgPriKey:   cipherKkey,
	}

	res := false
	if err := yc.Call("ms.RegisterMsgPriKey", args, &res); err != nil {
		return err
	}

	return nil
}

func WarpClient(clt *rpc.Client, pi *peer.AddrInfo, pk crypto.PubKey, DstID peer.ID) (ci.YTHClient, error) {
	var yc = new(YTHostClient)
	yc.Client = clt
	yc.localPeerID = pi.ID
	yc.localPeerPubKey, _ = pk.Raw()

	rAddrInfo, err := yc.RemotePeer()
	if err != nil {
		return nil, err
	}
	yc.RemotePeerID = rAddrInfo.ID

	if DstID == peer.ID(0) {
		yc.DstID = yc.RemotePeerID
	}else {
		yc.DstID = DstID
	}

	//fmt.Printf("peer ID:[%s]\n", yc.RemotePeerID.String())

	//gener current session msg pri key
	msgprikey, _, _ := crypto.GenerateSecp256k1Key(rand.Reader)
	prikey,_ := msgprikey.Raw()
	yc.msgPriKey =  prikey

	err = yc.SendMsgPriKey()
	if err != nil {
		return nil, err
	}

	for _, v := range pi.Addrs {
		yc.localPeerAddrs = append(yc.localPeerAddrs, v.String())
	}
	yc.isClosed = false

	return yc, nil
}

func (yc *YTHostClient) SendMsg(ctx context.Context, pid peer.ID, id int32, data []byte) ([]byte, error) {

	resChan := make(chan service.Response)
	errChan := make(chan error)

	defer func() {
		if err := recover(); err != nil {
			errChan <- err.(error)
		}
	}()

	aesKey := yc.msgPriKey

	//fmt.Printf("secret key [%s] ------>  before at aes msg: [%s]\n", base58.Encode(aesKey), string(data))

	aesData, err := encrypt.AesCBCEncrypt(data, aesKey)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("after at aes data: %s\n", string(aesData))

	go func() {
		var res service.Response

		pi := service.PeerInfo{yc.localPeerID, yc.localPeerAddrs, yc.localPeerPubKey}

		if err := yc.Call("ms.HandleMsg",
			service.Request{id, aesData, pi, pid}, &res); err != nil {
			errChan <- err
		} else {
			resChan <- res
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("relay ctx time out")
	case rd := <-resChan:
		rpsData, err := encrypt.AesCBCDncrypt(rd.Data, aesKey)
		if err != nil {
			err =  fmt.Errorf("respones AesCBCDncrypt msg error %x", err)
		}
		return rpsData, nil
	case err := <-errChan:
		return nil, err
	}
}

func (yc *YTHostClient) Ping(ctx context.Context) bool {

	successChan := make(chan struct{})
	errorChan := make(chan struct{})

	defer func() {
		if err := recover(); err != nil {
			errorChan <- struct{}{}
		}
	}()

	go func() {
		var res string
		if err := yc.Call("ms.Ping", "ping", &res); err != nil {
			errorChan <- struct{}{}
		} else if string(res) != "pong" {
			errorChan <- struct{}{}
		}
		successChan <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return false
	case <-errorChan:
		return false
	case <-successChan:
		return true
	}
}

func (yc *YTHostClient) Close() error {
	yc.isClosed = true
	return yc.Client.Close()
}

func (yc *YTHostClient) IsClosed() bool {
	return yc.isClosed
}

func (yc *YTHostClient) SendMsgClose(ctx context.Context, pid peer.ID, id int32, data []byte) ([]byte, error) {
	defer yc.Close()
	return yc.SendMsg(ctx, pid, id, data)
}

func (yc *YTHostClient) GetRemotePeerID() peer.ID {
	return yc.RemotePeerID
}
