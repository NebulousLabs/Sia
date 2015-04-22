package gateway

import (
	"net"
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

var pong = [4]byte{'p', 'o', 'n', 'g'}

// Ping returns whether an Address is reachable and responds correctly to the
// ping request -- in other words, whether it is a potential peer.
func (g *Gateway) Ping(addr modules.NetAddress) bool {
	var resp [4]byte
	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return false
	}
	if err := encoding.WriteObject(conn, handlerName("Ping")); err != nil {
		return false
	}
	if err := encoding.ReadObject(conn, &resp, 4); err != nil {
		return false
	}

	return err == nil && resp == pong
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func (g *Gateway) getExternalIP() error {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	hostname := string(buf[:n-1]) // trim newline

	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	g.myAddr = modules.NetAddress(net.JoinHostPort(hostname, g.myAddr.Port()))
	g.log.Println("INFO: set hostname to", g.myAddr)
	return nil
}
