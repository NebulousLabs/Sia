package network

import (
	"errors"
	"math/rand"
	"net"
	"net/http" // for getExternalIP()
	"sync"
	"time"
)

const (
	timeout   = time.Second * 5
	maxMsgLen = 1 << 24
)

var (
	ErrNoPeers = errors.New("no peers")
)

// An Address contains the information needed to contact a peer over TCP.
type Address string

// handlerName truncates a string to 8 bytes. If len(name) < 8, the remaining
// bytes are 0.
func handlerName(name string) []byte {
	b := make([]byte, 8)
	copy(b, name)
	return b
}

// Call establishes a TCP connection to the Address, calls the provided
// function on it, and closes the connection.
func (na Address) Call(name string, fn func(net.Conn) error) error {
	conn, err := net.DialTimeout("tcp", string(na), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	// set default deadline
	// note: fn can extend this deadline as needed
	conn.SetDeadline(time.Now().Add(timeout))
	// write header
	if _, err := conn.Write(handlerName(name)); err != nil {
		return err
	}
	return fn(conn)
}

// TBD
var BootstrapPeers = []Address{
	"23.239.14.98:9988",
}

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr      Address
	addressbook map[Address]struct{}
	handlerMap  map[string]func(net.Conn) error
	// used to protect addressbook and handlerMap
	sync.RWMutex
}

func (tcps *TCPServer) Address() Address {
	return tcps.myAddr
}

// NewTCPServer creates a TCPServer that listens on the specified address.
func NewTCPServer(addr string) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:    tcpServ,
		myAddr:      Address(addr),
		addressbook: make(map[Address]struct{}),
		handlerMap:  make(map[string]func(net.Conn) error),
	}
	// default handlers
	tcps.Register("SendHostname", sendHostname)
	tcps.Register("SharePeers", tcps.sharePeers)
	tcps.Register("AddMe", tcps.addRemote)

	// spawn listener
	go tcps.listen()
	return
}

// Bootstrap discovers the external IP of the TCPServer, requests peers from
// the initial peer list, and announces itself to those peers.
func (tcps *TCPServer) Bootstrap() (err error) {
	// populate initial peer list
	for _, addr := range BootstrapPeers {
		if tcps.Ping(addr) {
			tcps.AddPeer(addr)
		}
	}
	if len(tcps.addressbook) == 0 {
		// fallback to centralized service to learn hostname
		tcps.getExternalIP()
		return ErrNoPeers
	}

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	var peers []Address
	for _, addr := range tcps.AddressBook() {
		var resp []Address
		addr.RPC("SharePeers", nil, &resp)
		peers = append(peers, resp...)
	}
	for _, addr := range peers {
		if addr != tcps.myAddr && tcps.Ping(addr) {
			tcps.AddPeer(addr)
		}
	}

	// learn hostname
	var set bool
	for _, addr := range tcps.AddressBook() {
		var hostname string
		if err := addr.RPC("SendHostname", nil, &hostname); err == nil {
			tcps.setHostname(hostname)
			set = true
			break
		}
	}
	// if no peers respond, fallback to centralized service
	if !set {
		tcps.getExternalIP()
	}

	// announce ourselves to new peers
	tcps.Broadcast("AddMe", tcps.myAddr, nil)

	return
}

func (tcps *TCPServer) AddressBook() (book []Address) {
	tcps.RLock()
	defer tcps.RUnlock()
	for address := range tcps.addressbook {
		book = append(book, address)
	}
	return
}

// AddPeer safely adds a peer to the address book.
func (tcps *TCPServer) AddPeer(addr Address) {
	tcps.Lock()
	tcps.addressbook[addr] = struct{}{}
	tcps.Unlock()
}

// Remove safely removes a peer from the address book.
func (tcps *TCPServer) RemovePeer(addr Address) {
	tcps.Lock()
	delete(tcps.addressbook, addr)
	tcps.Unlock()
}

// RandomPeer selects and returns a random peer from the address book.
func (tcps *TCPServer) RandomPeer() Address {
	addrs := tcps.AddressBook()
	if len(addrs) == 0 {
		panic(ErrNoPeers)
	}
	return addrs[rand.Intn(len(addrs))]
}

// Ping returns whether a Address is reachable. It accomplishes this by
// initiating a TCP connection and immediately closes it. This is pretty
// unsophisticated. I'll add a Pong later.
func (tcps *TCPServer) Ping(addr Address) bool {
	conn, err := net.DialTimeout("tcp", string(addr), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// setHostname concatenates the supplied hostname with the current port.
func (tcps *TCPServer) setHostname(host string) {
	_, port, _ := net.SplitHostPort(string(tcps.myAddr))
	tcps.myAddr = Address(net.JoinHostPort(host, port))
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
		// no wait, send the error?
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
	buf := make([]byte, 32)
	n, err := resp.Body.Read(buf)
	if err != nil {
		return
	}
	// TODO: validate IP?
	tcps.setHostname(string(buf[:n]))
	return
}
