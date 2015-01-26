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
	timeout   = time.Second * 10
	maxMsgLen = 1 << 24
)

var (
	ErrNoPeers = errors.New("no peers")

	// hard-coded addresses used when bootstrapping
	BootstrapPeers = []Address{
		"23.239.14.98:9988",
	}
)

// An Address contains the information needed to contact a peer over TCP.
type Address string

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

// Address returns the Address of the server.
func (tcps *TCPServer) Address() Address {
	return tcps.myAddr
}

// AddressBook returns the server's address book as a slice.
func (tcps *TCPServer) AddressBook() []Address {
	tcps.RLock()
	defer tcps.RUnlock()
	book := []Address{}
	for address := range tcps.addressbook {
		book = append(book, address)
	}
	return book
}

// setHostname sets the hostname of the server. The port is unchanged. If we
// can't ping ourselves using the new hostname, setHostname returns false and
// the hostname is unchanged.
func (tcps *TCPServer) setHostname(host string) bool {
	_, port, _ := net.SplitHostPort(string(tcps.myAddr))
	newAddr := Address(net.JoinHostPort(host, port))
	// try to ping ourselves
	if !Ping(newAddr) {
		return false
	}
	tcps.myAddr = newAddr
	return true
}

// AddPeer safely adds a peer to the address book.
func (tcps *TCPServer) AddPeer(addr Address) error {
	tcps.Lock()
	defer tcps.Unlock()
	if _, exists := tcps.addressbook[addr]; exists {
		return errors.New("Peer already added")
	}
	tcps.addressbook[addr] = struct{}{}
	return nil
}

// Remove safely removes a peer from the address book.
func (tcps *TCPServer) RemovePeer(addr Address) error {
	tcps.Lock()
	defer tcps.Unlock()
	if _, exists := tcps.addressbook[addr]; !exists {
		return errors.New("No record of that peer")
	}
	delete(tcps.addressbook, addr)
	return nil
}

// RandomPeer selects and returns a random peer from the address book.
func (tcps *TCPServer) RandomPeer() (Address, error) {
	addrs := tcps.AddressBook()
	if len(addrs) == 0 {
		return "", ErrNoPeers
	}
	return addrs[rand.Intn(len(addrs))], nil
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
	set := tcps.setHostname(hostname)
	if !set {
		return errors.New("external hostname " + hostname + " did not respond to ping")
	}
	return
}

// Bootstrap discovers the external IP of the TCPServer, requests peers from
// the initial peer list, and announces itself to those peers.
func (tcps *TCPServer) Bootstrap() (err error) {
	// populate initial peer list
	for _, addr := range BootstrapPeers {
		if Ping(addr) {
			tcps.AddPeer(addr)
		}
	}
	if len(tcps.addressbook) == 0 {
		// no bootstrap nodes were reachable, so fallback to centralized
		// service to learn hostname
		tcps.getExternalIP()
		return ErrNoPeers
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

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	var peers []Address
	for _, addr := range tcps.AddressBook() {
		var resp []Address
		addr.RPC("SharePeers", nil, &resp)
		peers = append(peers, resp...)
	}
	for _, addr := range peers {
		if addr != tcps.myAddr && Ping(addr) {
			tcps.AddPeer(addr)
		}
	}

	// announce ourselves to new peers
	tcps.Broadcast("AddMe", tcps.myAddr, nil)

	return
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
	// default handlers (defined in handlers.go)
	tcps.RegisterRPC("Ping", pong)
	tcps.RegisterRPC("SendHostname", sendHostname)
	tcps.RegisterRPC("SharePeers", tcps.sharePeers)
	tcps.RegisterRPC("AddMe", tcps.addRemote)

	// spawn listener
	go tcps.listen()
	return
}
