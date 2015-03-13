package gateway

import (
	"io/ioutil"
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
	err := g.RPC(addr, "Ping", readerRPC(&resp, 4))
	return err == nil && resp == pong
}

// sendHostname replies to the sender with the sender's external IP.
func sendHostname(conn modules.NetConn) error {
	return conn.WriteObject(conn.Addr().Host())
}

func (g *Gateway) learnHostname(addr modules.NetAddress) error {
	var hostname string
	err := g.RPC(addr, "SendHostname", readerRPC(&hostname, 50))
	if err != nil {
		return err
	}
	g.setHostname(hostname)
	return nil
}

// setHostname sets the hostname of the server.
func (g *Gateway) setHostname(host string) {
	counter := g.mu.Lock()
	defer g.mu.Unlock(counter)
	g.myAddr = modules.NetAddress(net.JoinHostPort(host, g.myAddr.Port()))
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func (g *Gateway) getExternalIP() (err error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	hostname := string(buf[:n-1]) // trim newline
	// TODO: try to ping ourselves
	g.setHostname(hostname)
	return
}

func (g *Gateway) save(filename string) error {
	peers := g.Info().Peers
	return ioutil.WriteFile(filename, encoding.Marshal(peers), 0666)
}

func (g *Gateway) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	var peers []modules.NetAddress
	err = encoding.Unmarshal(contents, &peers)
	if err != nil {
		return
	}
	for _, peer := range peers {
		g.addPeer(peer)
	}
	return
}
