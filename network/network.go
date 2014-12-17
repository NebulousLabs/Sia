package network

import (
	"errors"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

const (
	timeout   = time.Second * 5
	maxMsgLen = 1 << 24
)

// A NetAddress contains the information needed to contact a peer over TCP.
type NetAddress struct {
	Host string
	Port uint16
}

// String returns the NetAddress as a string, concatentating the hostname and
// port number.
func (na *NetAddress) String() string {
	return net.JoinHostPort(na.Host, strconv.Itoa(int(na.Port)))
}

// handlerName truncates a string to 8 bytes. If len(name) < 8, the remaining
// bytes are 0.
func handlerName(name string) []byte {
	b := make([]byte, 8)
	copy(b, name)
	return b
}

// Call establishes a TCP connection to the NetAddress, calls the provided
// function on it, and closes the connection.
func (na *NetAddress) Call(name string, fn func(net.Conn) error) error {
	conn, err := net.DialTimeout("tcp", na.String(), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	// set default deadline
	// note: fn can extend this deadline as needed
	conn.SetDeadline(time.Now().Add(timeout))
	// write header
	conn.Write(handlerName(name))
	return fn(conn)
}

// TBD
var BootstrapPeers = []NetAddress{
	{"23.239.14.98", 9988},
}

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr      NetAddress
	addressbook map[NetAddress]struct{}
	handlerMap  map[string]func(net.Conn, []byte) error
	// used to protect addressbook and handlerMap
	sync.RWMutex
}

func (tcps *TCPServer) NetAddress() NetAddress {
	return tcps.myAddr
}

// NewTCPServer creates a TCPServer that listens on the specified port.
func NewTCPServer(port uint16) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:    tcpServ,
		myAddr:      NetAddress{"localhost", port},
		addressbook: make(map[NetAddress]struct{}),
		handlerMap:  make(map[string]func(net.Conn, []byte) error),
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
		return errors.New("can't bootstrap: no peers responded to ping")
	}

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	var peers []NetAddress
	for _, addr := range tcps.AddressBook() {
		var resp []NetAddress
		addr.RPC("SharePeers", nil, &resp)
		peers = append(peers, resp...)
	}
	for _, addr := range peers {
		if addr != tcps.myAddr && tcps.Ping(addr) {
			tcps.AddPeer(addr)
		}
	}

	// learn hostname
	for _, addr := range tcps.AddressBook() {
		var hostname string
		if err := addr.RPC("SendHostname", nil, &hostname); err == nil {
			tcps.myAddr.Host = hostname
			break
		}
	}
	// TODO: if hostname discovery fails, ask GetMyExternalIP

	// announce ourselves to new peers
	tcps.Broadcast("AddMe", tcps.myAddr.Port, nil)

	return
}

func (tcps *TCPServer) AddressBook() (book []NetAddress) {
	tcps.RLock()
	defer tcps.RUnlock()
	for address := range tcps.addressbook {
		book = append(book, address)
	}
	return
}

// AddPeer safely adds a peer to the address book.
func (tcps *TCPServer) AddPeer(addr NetAddress) {
	tcps.Lock()
	tcps.addressbook[addr] = struct{}{}
	tcps.Unlock()
}

// Remove safely removes a peer from the address book.
func (tcps *TCPServer) RemovePeer(addr NetAddress) {
	tcps.Lock()
	delete(tcps.addressbook, addr)
	tcps.Unlock()
}

// RandomPeer selects and returns a random peer from the address book.
func (tcps *TCPServer) RandomPeer() NetAddress {
	addrs := tcps.AddressBook()
	if len(addrs) == 0 {
		panic("no peers!")
	}
	return addrs[rand.Intn(len(addrs))]
}

// Ping returns whether a NetAddress is reachable. It accomplishes this by
// initiating a TCP connection and immediately closes it. This is pretty
// unsophisticated. I'll add a Pong later.
func (tcps *TCPServer) Ping(addr NetAddress) bool {
	conn, err := net.DialTimeout("tcp", addr.String(), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
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

// handleConn reads header data from a connection, unmarshals the data
// structures it contains, and routes the data to other functions for
// processing.
// TODO: set deadlines?
func (tcps *TCPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	ident := make([]byte, 8)
	if n, err := conn.Read(ident); err != nil || n != len(ident) {
		// TODO: log error
		return
	}
	msgData, err := encoding.ReadPrefix(conn, maxMsgLen)
	if err != nil {
		// TODO: log error
		return
	}
	// call registered handler for this message type
	tcps.RLock()
	fn, ok := tcps.handlerMap[string(ident)]
	tcps.RUnlock()
	if ok {
		fn(conn, msgData)
		// TODO: log error
		// no wait, send the error?
	}
	return
}
