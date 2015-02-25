package gateway

import (
	"net"
	"net/http" // for getExternalIP()
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	timeout   = time.Second * 10
	maxMsgLen = 1 << 24
)

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr     modules.NetAddress
	handlerMap map[string]func(net.Conn) error
	// used to protect addressbook and handlerMap
	sync.RWMutex
}

// Address returns the NetAddress of the server.
func (tcps *TCPServer) Address() modules.NetAddress {
	tcps.RLock()
	defer tcps.RUnlock()
	return tcps.myAddr
}

// setHostname sets the hostname of the server.
func (tcps *TCPServer) setHostname(host string) {
	tcps.Lock()
	defer tcps.Unlock()
	tcps.myAddr = modules.NetAddress(net.JoinHostPort(host, tcps.myAddr.Port()))
}

// listen runs in the background, accepting incoming connections and serving
// them. listen will return after TCPServer.Close() is called, because the
// Accept() call will fail.
func (tcps *TCPServer) listen() {
	for {
		conn, err := tcps.Accept()
		if err != nil {
			return
		}

		// set default deadline
		// note: the handler can extend this deadline as needed
		conn.SetDeadline(time.Now().Add(timeout))

		// it is the handler's responsibility to close the connection
		go tcps.handleConn(conn)
	}
}

// handleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (tcps *TCPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	ident := make([]byte, 8)
	if n, err := conn.Read(ident); err != nil || n != len(ident) {
		// TODO: log error
		return
	}
	// call registered handler for this message type
	tcps.RLock()
	fn, ok := tcps.handlerMap[string(ident)]
	tcps.RUnlock()
	if ok {
		fn(conn)
		// TODO: log error
	}
	return
}

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func (tcps *TCPServer) getExternalIP() (err error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	hostname := string(buf[:n-1]) // trim newline
	// TODO: try to ping ourselves
	tcps.setHostname(hostname)
	return
}

// newTCPServer creates a TCPServer that listens on the specified address.
func newTCPServer(addr string) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:   tcpServ,
		myAddr:     modules.NetAddress(addr),
		handlerMap: make(map[string]func(net.Conn) error),
	}
	// default handlers (defined in handlers.go)
	tcps.RegisterRPC("Ping", pong)
	tcps.RegisterRPC("SendHostname", sendHostname)

	// spawn listener
	go tcps.listen()
	return
}
