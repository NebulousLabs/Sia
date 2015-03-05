package gateway

import (
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// Ping returns whether an Address is reachable and responds correctly to the
// ping request -- in other words, whether it is a potential peer.
func (g *Gateway) Ping(addr modules.NetAddress) bool {
	var pong string
	err := g.RPC(addr, "Ping", modules.ReaderRPC(&pong, 4))
	if err != nil {
		println(err.Error())
	}
	return err == nil && pong == "pong"
}

func pong(conn modules.NetConn) error {
	return conn.WriteObject("pong")
}

// sendHostname replies to the sender with the sender's external IP.
func sendHostname(conn modules.NetConn) error {
	return conn.WriteObject(conn.Addr().Host())
}

func (g *Gateway) learnHostname(addr modules.NetAddress) error {
	var hostname string
	err := g.RPC(addr, "SendHostname", modules.ReaderRPC(&hostname, 50))
	if err != nil {
		return err
	}
	g.setHostname(hostname)
	return nil
}

// setHostname sets the hostname of the server.
func (g *Gateway) setHostname(host string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.myAddr = modules.NetAddress(net.JoinHostPort(host, g.myAddr.Port()))
}

// threadedBroadcast calls an RPC on all of the peers in the Gateway's peer
// list. The calls are run in parallel.
func (g *Gateway) threadedBroadcast(name string, fn modules.RPCFunc) {
	var badpeers []modules.NetAddress
	var wg sync.WaitGroup
	wg.Add(len(g.peers))

	g.mu.RLock()
	for peer := range g.peers {
		// contact each peer in a separate thread
		go func(peer modules.NetAddress) {
			err := g.RPC(peer, name, fn)
			// TODO: some errors will be our fault. Need to distinguish them.
			if err != nil {
				badpeers = append(badpeers, peer)
			}
			wg.Done()
		}(peer)
	}
	g.mu.RUnlock()
	wg.Wait()

	// process the bad peers
	g.mu.Lock()
	for _, peer := range badpeers {
		g.peers[peer]++ // increment strikes
		if g.peers[peer] > maxStrikes {
			g.removePeer(peer)
		}
	}
	g.mu.Unlock()
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
