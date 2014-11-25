package network

import (
	"net"
	"strconv"
	"time"
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

// Call establishes a TCP connection to the NetAddress, calls the provided
// function on it, and closes the connection.
func (na *NetAddress) Call(fn func(net.Conn) error) error {
	conn, err := net.DialTimeout("tcp", na.String(), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	return fn(conn)
}

// TBD
var BootstrapPeers = []NetAddress{
// {"10.0.0.5", 9988},
// {"10.0.0.11", 9988},
// {"10.0.0.12", 9988},
}

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr      NetAddress
	addressbook map[NetAddress]struct{}
	handlerMap  map[byte]func(net.Conn, []byte) error
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
		myAddr:      NetAddress{"", port},
		addressbook: make(map[NetAddress]struct{}),
		handlerMap:  make(map[byte]func(net.Conn, []byte) error),
	}
	// default handlers
	tcps.Register('H', sendHostname)
	tcps.Register('P', tcps.sharePeers)
	tcps.Register('A', tcps.addPeer)

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
			tcps.addressbook[addr] = struct{}{}
		}
	}

	// learn hostname
	for addr := range tcps.addressbook {
		var hostname string
		if addr.RPC('H', nil, &hostname) == nil {
			tcps.myAddr.Host = hostname
			break
		}
	}

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	var peers []NetAddress
	for addr := range tcps.addressbook {
		var resp []NetAddress
		addr.RPC('P', nil, &resp)
		peers = append(peers, resp...)
	}
	for _, addr := range peers {
		if addr != tcps.myAddr && tcps.Ping(addr) {
			tcps.addressbook[addr] = struct{}{}
		}
	}

	// announce ourselves to new peers
	tcps.Announce('A', tcps.myAddr)

	return
}

func (tcps *TCPServer) AddressBook() (book []NetAddress) {
	for address := range tcps.addressbook {
		book = append(book, address)
	}
	return
}

// RandomPeer selects and returns a random peer from the address book.
// TODO: probably not smart to depend on map iteration...
func (tcps *TCPServer) RandomPeer() (rand NetAddress) {
	for addr := range tcps.addressbook {
		rand = addr
		break
	}
	return
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

// Broadcast calls the specified function on each peer in the address book.
func (tcps *TCPServer) Broadcast(fn func(net.Conn) error) {
	for addr := range tcps.addressbook {
		addr.Call(fn)
	}
}

// Announce sends an object to every peer in the address book.
func (tcps *TCPServer) Announce(t byte, obj interface{}) {
	for addr := range tcps.addressbook {
		addr.Call(func(conn net.Conn) error {
			conn.Write([]byte{t})
			_, err := WriteObject(conn, obj)
			return err
		})
	}
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
	msgType := make([]byte, 1)
	if n, err := conn.Read(msgType); err != nil || n != 1 {
		// TODO: log error
		return
	}
	msgData, err := ReadPrefix(conn)
	if err != nil {
		// TODO: log error
		return
	}
	// call registered handler for this message type
	if fn, ok := tcps.handlerMap[msgType[0]]; ok {
		fn(conn, msgData)
		// TODO: log error
		// no wait, send the error?
	}
	return
}
