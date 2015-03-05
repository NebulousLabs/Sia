package gateway

import (
	"net"
	"net/http" // for getExternalIP()

	"github.com/NebulousLabs/Sia/modules"
)

// Address returns the NetAddress of the server.
func (g *Gateway) Address() modules.NetAddress {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.myAddr
}

// setHostname sets the hostname of the server.
func (g *Gateway) setHostname(host string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.myAddr = modules.NetAddress(net.JoinHostPort(host, g.myAddr.Port()))
}

// listen runs in the background, accepting incoming connections and serving
// them. listen will return after TCPServer.Close() is called, because the
// accept call will fail.
func (g *Gateway) listen(addr string) (l net.Listener, err error) {
	l, err = net.Listen("tcp", addr)
	if err != nil {
		return
	}

	// listener process
	go func() {
		for {
			conn, err := accept(l)
			if err != nil {
				return
			}

			// it is the handler's responsibility to close the connection
			go g.handleConn(conn)
		}
	}()

	return
}

// handleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (g *Gateway) handleConn(conn modules.NetConn) {
	defer conn.Close()
	var id rpcID
	if err := conn.ReadObject(&id, 8); err != nil {
		// TODO: log error
		return
	}
	// call registered handler for this ID
	g.mu.RLock()
	fn, ok := g.handlerMap[id]
	g.mu.RUnlock()
	if ok {
		fn(conn)
		// TODO: log error
	}
	return
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
