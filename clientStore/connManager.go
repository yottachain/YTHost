package clientStore

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	ci "github.com/yottachain/YTHost/clientInterface"
	"github.com/yottachain/YTHost/util"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
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

type ClientStore struct {
	connect func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (ci.YTHClient, error)
	sync.Map
	l sync.Mutex	//对下面map进行加锁
	connTopid map[ci.YTHClient] []peer.ID //记录一个链接被多少个pid使用，如果通过中继建立的链接那么一个实际链接对应多个pid
}

// Get 获取一个客户端，如果没有，建立新的客户端连接
func (cs *ClientStore) Get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (ci.YTHClient, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("ctx done")
	default:
		return cs.get(ctx, pid, mas)
	}
}

func (cs *ClientStore) get(ctx context.Context, pid peer.ID, mas []multiaddr.Multiaddr) (ci.YTHClient, error) {

	// 尝试次数
	var tryCount int
	const max_try_count = 5

	// 取已存在clt
start:
	// 如果达到最大尝试次数就返回错误
	if tryCount++; tryCount > max_try_count {
		return nil, fmt.Errorf("Maximum attempts %d ", max_try_count)
	}
	_c, ok := cs.Map.Load(pid)
	// 如果不存在创建新的clt
	if !ok {
		relayID := util.GetRealyId(mas)
		if relayID.String() != "" {
			cli, ok := cs.Map.Load(relayID)

			if ok {
				c := cli.(ci.YTHClient)
				cs.Map.Store(pid, c)
				cs.StoreConnInfo(pid, c)
				return c, nil
			}
		}

		if clt, err := cs.connect(ctx, pid, mas); err != nil {
			return nil, err
		} else {
			cs.Map.Store(pid, clt)
			cs.StoreConnInfo(pid, clt)
			if relayID.String() != "" {
				cs.Map.Store(relayID, clt)
				cs.StoreConnInfo(relayID, clt)
			}
			// 创建clt完成后返回到开始
			goto start
		}
	} else {
		// 如果已存在clt无法ping通,删除记录重新创建
		c := _c.(ci.YTHClient)
		if c.IsClosed() || !c.Ping(ctx) {
			err := cs.DelConnInfo(pid, c)
			if err == nil {
				Error.Printf("ping fail--->peerid:%s close connect succeed\n", pid.String())
				cs.Map.Delete(pid)
			}else {
				Error.Printf("ping fail--->peerid:%s close connect error:%s\n", pid.String(), err)
			}
			goto start
		}

		return c, nil
	}
}

func (cs *ClientStore) GetByAddrString(ctx context.Context, id string, addrs []string) (ci.YTHClient, error) {
	/*buf, _ := base58.Decode(id)
	pid, err := peer.IDFromBytes(buf)*/
	pid, err := peer.Decode(id)
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

	return cs.get(ctx, pid, mas)
}

// Close 关闭一个客户端
func (cs *ClientStore) Close(pid peer.ID) error {
	_clt, ok := cs.Map.Load(pid)
	if !ok {
		Error.Printf("peerid:%s connect not exist\n", pid.String())
		return fmt.Errorf("no find client ID is %s", pid.Pretty())
	}
	clt := _clt.(ci.YTHClient)

	err := cs.DelConnInfo(pid, clt)
	if err == nil {
		Error.Printf("peerid:%s close connect succeed\n", pid.String())
		cs.Map.Delete(pid)
	}else {
		Error.Printf("peerid:%s close connect error:%s\n", pid.String(), err)
	}
	return err
}

func (cs *ClientStore) GetClient(pid peer.ID) (ci.YTHClient, bool) {

	_clt, ok := cs.Map.Load(pid)
	if ok {
		clt := _clt.(ci.YTHClient)
		return clt, ok
	}
	return nil, ok
}

func (cs *ClientStore) StoreConnInfo(pid peer.ID, clt ci.YTHClient) () {
	cs.l.Lock()
	defer cs.l.Unlock()
	pids, ok := cs.connTopid[clt]
	exist := 0

	if ok {
		for _, c_pid := range pids {
			if pid == c_pid {
				Error.Printf("peerid:%s connect exist\n", pid.String())
				exist = 1
				break
			}
		}
		if exist == 0 {
			Error.Printf("peerid:%s connect append ----\n", pid.String())
			cs.connTopid[clt] = append(pids, pid)
		}
	}else {
		Error.Printf("peerid:%s connect create ======\n", pid.String())
		pids = make([]peer.ID, 1)
		pids[0] = pid
		cs.connTopid[clt] = pids
	}
}

func (cs *ClientStore) DelConnInfo(pid peer.ID, clt ci.YTHClient) error {
	cs.l.Lock()
	defer cs.l.Unlock()
	pids, ok := cs.connTopid[clt]

	if ok {
		for i, c_pid := range pids {
			if pid == c_pid {
				pids = append(pids[:i], pids[i+1:]...)
				cs.connTopid[clt] = pids
				break
			}
		}

		if len(pids) == 0 {
			delete(cs.connTopid, clt)
			Error.Printf("peerid:%s close begin\n", pid.String())
			return clt.Close()
		}else {
			return nil
		}
	}else {
		//return fmt.Errorf("peer id matching clt error!")
		return nil
	}
}

func (cs *ClientStore) PrintConnInfo(clt ci.YTHClient) {
	cs.l.Lock()
	defer cs.l.Unlock()
	pids, ok := cs.connTopid[clt]
	if ok {
		lenth := len(pids)
		for i := 0; i < lenth; i++ {
			fmt.Println(pids[i].String())
		}
	}
}

// Len 返回当前连接数
//func (cs *ClientStore) Len() int {
//}

func NewClientStore(connFunc func(ctx context.Context, id peer.ID, mas []multiaddr.Multiaddr) (ci.YTHClient, error)) *ClientStore {
	return &ClientStore{
		connFunc,
		sync.Map{},
		sync.Mutex{},
		map[ci.YTHClient][]peer.ID{},
	}
}
