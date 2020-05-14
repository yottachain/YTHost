package host

import (
	"container/list"
	"context"
	"fmt"
	counter "github.com/yottachain/NodeOptimization/Counter"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"net/rpc"
	"os"
	"path"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	mnet "github.com/multiformats/go-multiaddr-net"
	"github.com/yottachain/NodeOptimization"
	"github.com/yottachain/YTHost/client"
	"github.com/yottachain/YTHost/clientStore"
	"github.com/yottachain/YTHost/config"
	"github.com/yottachain/YTHost/connAutoCloser"
	"github.com/yottachain/YTHost/option"
	"github.com/yottachain/YTHost/peerInfo"
	"github.com/yottachain/YTHost/service"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

//type Host interface {
//	Accept()
//	Addrs() []multiaddr.Multiaddr
//	Server() *rpc.Server
//	Config() *config.Config
//	Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error)
//	RegisterHandler(id service.MsgId, handlerFunc service.Handler)
//}

type host struct {
	cfg      *config.Config
	listener mnet.Listener
	srv      *rpc.Server
	service.HandlerMap
	clientStore *clientStore.ClientStore
	ow          *optWarp
}

func NewHost(options ...option.Option) (*host, error) {
	hst := new(host)
	hst.ow = &optWarp{optimizer.New(), nil, time.Time{}, sync.RWMutex{}}
	hst.ow.Optmizer.GetScore = optGetScore

	go hst.ow.Run(context.Background())

	// 打印计数器
	go func() {
		for {
			<-time.After(time.Minute)
			logpath := path.Join(path.Dir(os.Args[0]), "opt.log")
			fl, err := os.OpenFile(logpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				continue
			}

			for k, v := range hst.ow.Optmizer.CurrentCount(NodeIds...) {
				fmt.Fprintf(fl, "%s,%d,%d,%d,%d", k, v.SuccTimes, v.FailTimes, v.AvgDelayTimes, v.Score)
			}
		}
	}()

	hst.cfg = config.NewConfig()

	for _, bindOp := range options {
		bindOp(hst.cfg)
	}

	ls, err := mnet.Listen(hst.cfg.ListenAddr)

	if err != nil {
		return nil, err
	}

	hst.listener = ls

	srv := rpc.NewServer()
	hst.srv = srv

	hst.HandlerMap = make(service.HandlerMap)

	hst.clientStore = clientStore.NewClientStore(hst.Connect)

	if hst.cfg.PProf != "" {
		go func() {
			if err := http.ListenAndServe(hst.cfg.PProf, nil); err != nil {
				fmt.Println("PProf open fail:", err)
			} else {
				fmt.Println("PProf debug open:", hst.cfg.PProf)
			}
		}()
	}

	return hst, nil
}

func (hst *host) Accept() {
	addrService := new(service.AddrService)
	addrService.Info.ID = hst.cfg.ID
	addrService.Info.Addrs = hst.Addrs()
	addrService.PubKey = hst.Config().Privkey.GetPublic()

	msgService := new(service.MsgService)
	msgService.Handler = hst.HandlerMap
	msgService.Pi = peerInfo.PeerInfo{hst.cfg.ID, hst.Addrs()}

	if err := hst.srv.RegisterName("as", addrService); err != nil {
		panic(err)
	}

	if err := hst.srv.RegisterName("ms", msgService); err != nil {
		panic(err)
	}

	//for {
	//	hst.srv.Accept(mnet.NetListener(hst.listener))
	//}

	lis := mnet.NetListener(hst.listener)
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Print("rpc.Serve: accept:", err.Error())
			continue
		}
		ac := connAutoCloser.New(conn)
		ac.SetOuttime(time.Minute * 5)
		go hst.srv.ServeConn(ac)
	}
}

func (hst *host) Listenner() mnet.Listener {
	return hst.listener
}

func (hst *host) Server() *rpc.Server {
	return hst.srv
}

func (hst *host) Config() *config.Config {
	return hst.cfg
}

func (hst *host) ClientStore() *clientStore.ClientStore {
	return hst.clientStore
}

func (hst *host) Addrs() []multiaddr.Multiaddr {

	port, err := hst.listener.Multiaddr().ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return nil
	}

	tcpMa, err := multiaddr.NewMultiaddr(fmt.Sprintf("/tcp/%s", port))

	if err != nil {
		return nil
	}

	var res []multiaddr.Multiaddr
	maddrs, err := mnet.InterfaceMultiaddrs()
	if err != nil {
		return nil
	}

	for _, ma := range maddrs {
		newMa := ma.Encapsulate(tcpMa)
		if mnet.IsIPLoopback(newMa) {
			continue
		}
		res = append(res, newMa)
	}
	return res
}

// Connect 连接远程节点
func (hst *host) Connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (*client.YTHostClient, error) {
	var status int
	var interval int64
	defer func() {
		//  标记成功失败
		hst.ow.Feedback(counter.InRow{pid.Pretty(), status, interval})
	}()

	startTime := time.Now()
	conn, err := hst.connect(ctx, pid, mas)
	interval = time.Now().Sub(startTime).Milliseconds()
	if err != nil {
		status = 1
		return nil, err
	}

	clt := rpc.NewClient(conn)
	ytclt, err := client.WarpClient(clt, &peer.AddrInfo{
		hst.cfg.ID,
		hst.Addrs(),
	}, hst.cfg.Privkey.GetPublic())
	if err != nil {
		status = 1
		return nil, err
	}
	return ytclt, nil
}

func (hst *host) connect(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (mnet.Conn, error) {
	connChan := make(chan mnet.Conn)
	errChan := make(chan error)
	wg := sync.WaitGroup{}
	wg.Add(len(mas))

	go func() {
		wg.Wait()
		select {
		case errChan <- fmt.Errorf("dail all maddr fail"):
		case <-time.After(time.Millisecond * 500):
			return
		}
	}()

	for _, addr := range mas {
		go func(addr multiaddr.Multiaddr) {
			defer wg.Done()
			d := &mnet.Dialer{}
			if conn, err := d.DialContext(ctx, addr); err == nil {
				select {
				case connChan <- conn:
				case <-time.After(time.Second * 30):
				}
			} else {
				if hst.cfg.Debug {
					log.Println("conn error:", err)
				}
			}
		}(addr)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("ctx quit")
		case conn := <-connChan:
			return conn, nil
		case err := <-errChan:
			return nil, err
		}
	}
}

// ConnectAddrStrings 连接字符串地址
func (hst *host) ConnectAddrStrings(ctx context.Context, id string, addrs []string) (*client.YTHostClient, error) {

	buf, _ := base58.Decode(id)
	pid, err := peer.IDFromBytes(buf)
	if err != nil {
		return nil, err
	}

	var mas = make([]multiaddr.Multiaddr, len(addrs))
	for k, v := range addrs {
		ma, err := multiaddr.NewMultiaddr(v)
		if err != nil {
			continue
		}
		mas[k] = ma
	}

	return hst.Connect(ctx, pid, mas)
}

// SendMsg 发送消息
func (hst *host) SendMsg(ctx context.Context, pid peer.ID, mid int32, msg []byte) ([]byte, error) {

	clt, ok := hst.ClientStore().GetClient(pid)
	if !ok {
		return nil, fmt.Errorf("no client ID is:%s", pid.Pretty())
	}

	var status int
	var interval int64
	defer func() {
		//  标记成功失败
		hst.ow.Feedback(counter.InRow{pid.Pretty(), status, interval})
	}()

	startTime := time.Now()
	res, err := clt.SendMsg(ctx, mid, msg)
	interval = time.Now().Sub(startTime).Milliseconds()
	if err != nil {
		status = 1
		return nil, err
	}

	//res, err := clt.SendMsg(ctx, mid, msg)
	return res, err
}

type optWarp struct {
	*optimizer.Optmizer
	nodes      []string
	preGetTime time.Time
	mtx        sync.RWMutex
}

func (ow *optWarp) GetNodes(ids []string, optNum int, randNum int) []string {
	if time.Now().Sub(ow.preGetTime) > time.Second {
		ow.mtx.Lock()
		ow.nodes = ow.getNodes(ids, optNum, randNum)
		ow.preGetTime = time.Now()
		ow.mtx.Unlock()
	}
	return ow.nodes
}

func (ow *optWarp) getNodes(ids []string, optNum int, randNum int) []string {

	var res []string

	nodes := list.New()
	//optlist := ow.Optmizer.Get2(NodeIds...)
	optlist := ow.Optmizer.Get2(ids...)

	for i := 0; i < optNum; i++ {
		nodes.PushBack(optlist[i])
	}

	// 插入随机节点
	for i := 0; i < randNum; i++ {
		node := optlist[optNum+rand.Intn(len(optlist)-1-optNum)]
		pos := rand.Intn(nodes.Len())

		curr := nodes.Front()
		for j := 0; j < pos; j++ {
			curr = curr.Next()
		}
		nodes.InsertAfter(node, curr)
	}

	// list转数组
	res = make([]string, nodes.Len())
	curr := nodes.Front()
	i := 0
	for {
		res[i] = curr.Value.(string)

		curr = curr.Next()
		if curr == nil {
			break
		}
		i = i + 1
	}

	fmt.Println("返回节点", len(res))
	return res
}

func (hst *host) GetNodes(ids []string, optNum int, randNum int) []string {
	return hst.ow.GetNodes(ids, optNum, randNum)
}

func optGetScore(row counter.NodeCountRow) int64 {
	if (row.SuccTimes + row.FailTimes)==0 {
		return 500
	}
	total := row.SuccTimes + row.FailTimes
	rate := float32(row.SuccTimes) / float32(total)
	return 500 + int64(1000*rate)
}
//
//func optGetScore1(row counter.NodeCountRow) int64 {
//	if (row[0]+row[1])==0 {
//		return optGetScore(row)
//	}
//
//	return 30000 - row[2] + optGetScore(row)
//}