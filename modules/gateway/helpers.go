package gateway

import (
	"net"
	"net/http"

	"github.com/NebulousLabs/Sia/modules"
)

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
