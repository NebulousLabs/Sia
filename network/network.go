package network

import (
	"errors"
	"net"
	"net/http" // for getExternalIP()
	"sync"
	"time"
)

const (
	timeout   = time.Second * 10
	maxMsgLen = 1 << 24
)

// An Address contains the information needed to contact a peer over TCP.
type Address string

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr     Address
	handlerMap map[string]func(net.Conn) error
	// used to protect addressbook and handlerMap
	sync.RWMutex
}

// Address returns the Address of the server.
func (tcps *TCPServer) Address() Address {
	tcps.RLock()
	defer tcps.RUnlock()
	return tcps.myAddr
}

// setHostname sets the hostname of the server. The port is unchanged. If we
// can't ping ourselves using the new hostname, setHostname returns false and
// the hostname is unchanged.
func (tcps *TCPServer) setHostname(host string) error {
	_, port, _ := net.SplitHostPort(string(tcps.myAddr))
	newAddr := Address(net.JoinHostPort(host, port))
	// try to ping ourselves
	if !Ping(newAddr) {
		return errors.New("supplied hostname was unreachable")
	}
	tcps.Lock()
	tcps.myAddr = newAddr
	tcps.Unlock()
	return nil
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
	err = tcps.setHostname(hostname)
	return
}

// Bootstrap discovers the external IP of the TCPServer, requests peers from
// the initial peer list, and announces itself to those peers.
func (tcps *TCPServer) Bootstrap(bootstrapPeer Address) (err error) {
	// if bootstrapPeer is reachable, ask it for our hostname
	var hostname string
	if Ping(bootstrapPeer) && bootstrapPeer.RPC("SendHostname", nil, &hostname) == nil {
		err = tcps.setHostname(hostname)
		return
	}

	// otherwise, fallback to centralized service
	err = tcps.getExternalIP()
	if err != nil {
		return
	}

	return errors.New("unable to determine hostname")
}

// NewTCPServer creates a TCPServer that listens on the specified address.
func NewTCPServer(addr string) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:   tcpServ,
		myAddr:     Address(addr),
		handlerMap: make(map[string]func(net.Conn) error),
	}
	// default handlers (defined in handlers.go)
	tcps.RegisterRPC("Ping", pong)
	tcps.RegisterRPC("SendHostname", sendHostname)

	// spawn listener
	go tcps.listen()
	return
}
