package host

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"io/ioutil"
	"net/http"
)

var nodeListStr = `[
{
  "ID": "16Uiu2HAm7o24DSgWTrcu5sLCgSkf3D3DQqzpMz9W1Bi7F2Cc4SF6",
  "Addrs": ["/dns4/sn00.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmNe1bZF2s7msxqy9tFT7WDfUaJa98h1KBhAmTTHvcZqpA",
  "Addrs": ["/dns4/sn01.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAkyZAuzcjmpFhk1pCLAZaYusV3wXmrEhnnNDfeJjkVoQc6",
  "Addrs": ["/dns4/sn02.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmSFq7SbwcfYVn3NzWuuV7SizQEVjKEwty1knZuzTA7jDq",
  "Addrs": ["/dns4/sn03.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmC7DSN4kNi64sB5N9aMgv9DjTTrtydf4YKS3Q56hYsDNS",
  "Addrs": ["/dns4/sn04.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmSFgs5Pj6hFdAzCAvFGH78ew7egakT6VqL1xaLdvxnnSc",
  "Addrs": ["/dns4/sn05.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmP2RuNAkXdtQDiFqVuBA8yERh91JV6b29rQpAGKkb3PiM",
  "Addrs": ["/dns4/sn06.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmJQ7cjPzi7u4NdgYK7xWqDgEVBqAqNPrmxY2KVKGpND2W",
  "Addrs": ["/dns4/sn07.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmDKteRvgXPtzz3pvhGn56HH7uo8WqoGqJWPArY4G1kuWP",
  "Addrs": ["/dns4/sn08.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmDpQ6527dqtiv5fixTptQBtGa561BZeUTDuALiAZwQNGR",
  "Addrs": ["/dns4/sn09.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmATZsCop9hkKDbmtyLbizLQU92jrCVpWvzRChKRQbwzy7",
  "Addrs": ["/dns4/sn10.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmGheSFhwbpihhEnyZUxsVr6Rn9z5v2XDMeEyAfK2K4nwG",
  "Addrs": ["/dns4/sn11.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAm2JANbeeaXDa9JaDTU5Q1h2hmjJGJx91LpYd36pdoDWdx",
  "Addrs": ["/dns4/sn12.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAm3Cmzqg9TKR6FvEH5NSgzLZgDZb4xtPC9aYhqbc9p7WM5",
  "Addrs": ["/dns4/sn13.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAm2CzizQh2AU8NXK5z2bvJUaFuPiM9Z6R1uDEFKDvob4mJ",
  "Addrs": ["/dns4/sn14.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmTd1jqEGLThwcrD9yYG1JsHHj7qsDJDBcdgMLMvaBnksU",
  "Addrs": ["/dns4/sn15.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmHufUv4udcL1f1bNP4r6VqDBppmKH495iQKSgv6nWGoZA",
  "Addrs": ["/dns4/sn16.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmKS35S4JQk8BDUvgWhjGLMJ1f9zWJhT3QeRRyFdReXeue",
  "Addrs": ["/dns4/sn17.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmUyPbR4wcKtGi6n84CGkHsXsHZZ2sGrnhJPAqJmFCMfDW",
  "Addrs": ["/dns4/sn18.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmLUCp92e25HXiZW8fMwpCUfhQRcNGL7PibTDtg51JTRCq",
  "Addrs": ["/dns4/sn19.yottachain.net/tcp/9999"]
},
{
  "ID": "16Uiu2HAmBG1d8HHBApLg9MrDqgUX4LoKcFCSCrq54QW3mkqRheo1",
  "Addrs": ["/dns4/sn20.yottachain.net/tcp/9999"]
}
]`

type NodeInfo struct {
	ID    string   `json:"ID"`
	Addrs []string `json:"Addrs"`
}
type NodeInfo2 struct {
	ID    string `json:"nodeid"`
	Addrs string `json:"ip"`
}

func GetACNodeList() []*peer.AddrInfo {
	var ns []NodeInfo = make([]NodeInfo, 0)
	var ns2 []NodeInfo2 = make([]NodeInfo2, 0)

	resp, err := http.Get("http://39.105.184.162:8082/active_nodes")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	err = json.Unmarshal([]byte(nodeListStr), &ns)
	if err != nil {
		fmt.Println(err.Error())
	}
	err = json.Unmarshal(buf, &ns2)
	if err != nil {
		fmt.Println(err.Error())
	}

	var res = make([]*peer.AddrInfo, 0)

	for _, v := range ns {
		_res := &peer.AddrInfo{}
		var id peer.ID
		err := id.UnmarshalText([]byte(v.ID))
		if err != nil {
			continue
		}
		var addrs []multiaddr.Multiaddr
		for _, addrstr := range v.Addrs {
			ma, err := multiaddr.NewMultiaddr(addrstr)
			if err != nil {
				continue
			}
			addrs = append(addrs, ma)
		}

		_res.ID = id
		_res.Addrs = addrs
		res = append(res, _res)
	}

	for _, v := range ns2 {
		_res := &peer.AddrInfo{}
		var id peer.ID
		err := id.UnmarshalText([]byte(v.ID))
		if err != nil {
			continue
		}
		var addrs []multiaddr.Multiaddr
		ma, err := multiaddr.NewMultiaddr(v.Addrs)
		if err != nil {
			continue
		}
		addrs = append(addrs, ma)

		_res.ID = id
		_res.Addrs = addrs
		res = append(res, _res)
	}

	return res
}
