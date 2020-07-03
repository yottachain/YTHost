package httpHost
//
//import (
//	"context"
//	"github.com/libp2p/go-libp2p-core/peer"
//	"github.com/multiformats/go-multiaddr"
//	manet "github.com/multiformats/go-multiaddr-net"
//	"net/http"
//)
//
//type Transport struct {
//}
//
//type ConnRecord struct {
//	mas     []multiaddr.Multiaddr
//	connMap map[multiaddr.Multiaddr]manet.Conn
//}
//
//func (cp *ConnRecord) Connect(ctx context.Context, ma multiaddr.Multiaddr) error {
//	conn, err := manet.Dialer{}.DialContext(ctx, ma)
//	if err != nil {
//		return err
//	}
//
//	cp.connMap[ma] = conn
//	return nil
//}
//
//type ConnBook struct {
//	cpmap map[peer.ID]*ConnRecord
//}
//
//func (cp *ConnRecord) Close(ma multiaddr.Multiaddr) error {
//	defer delete(cp.connMap, ma)
//
//	return cp.connMap[ma].Close()
//}
//
//func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
//	return nil, nil
//}
