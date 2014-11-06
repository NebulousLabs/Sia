package sia

import (
	"errors"
	"net"
	"strconv"
	"time"
)

const (
	timeout   = time.Second * 5
	maxMsgLen = 1 << 16
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

// call establishes a TCP connection to the NetAddress, calls the provided
// function on it, and closes the connection.
func (na *NetAddress) Call(fn func(net.Conn) error) error {
	conn, err := net.Dial("tcp", na.String())
	if err != nil {
		return err
	}
	defer conn.Close()
	return fn(conn)
}

// TBD
var BootstrapPeers = []NetAddress{}

// A TCPServer sends and receives messages. It also maintains an address book
// of peers to broadcast to and make requests of.
type TCPServer struct {
	net.Listener
	myAddr      NetAddress
	addressbook map[NetAddress]struct{}
	handlerMap  map[byte]func(net.Conn, []byte) error
}

func NewTCPServer(port uint16) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:    tcpServ,
		addressbook: make(map[NetAddress]struct{}),
		handlerMap:  make(map[byte]func(net.Conn, []byte) error),
	}
	// default handlers
	tcps.handlerMap['H'] = tcps.sendHostname
	tcps.handlerMap['P'] = tcps.sendHostname

	// spawn listener
	go tcps.listen()
	return
}

// Register registers a message type with a message handler. The existing
// handler for that type will be overwritten.
func (tcps *TCPServer) Register(t byte, fn func(net.Conn, []byte) error) {
	tcps.handlerMap[t] = fn
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
func (tcps *TCPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	var (
		msgHead []byte = make([]byte, 5)
		msgData []byte // length determined by msgHead
	)
	if n, err := conn.Read(msgHead); err != nil || n != 5 {
		// TODO: log error
		return
	}
	msgLen := DecUint64(msgHead[1:])
	if msgLen > maxMsgLen {
		// TODO: log error
		return
	}
	msgData = make([]byte, msgLen)
	if n, err := conn.Read(msgData); err != nil || uint64(n) != msgLen {
		// TODO: log error
		return
	}

	// call registered handler for this message type
	if fn, ok := tcps.handlerMap[msgHead[0]]; ok {
		fn(conn, msgData)
		// TODO: log error
	}
	return
}

// sendHostname replies to the send with the sender's external IP.
func (tcps *TCPServer) sendHostname(conn net.Conn, _ []byte) error {
	_, err := conn.Write([]byte(conn.RemoteAddr().String()))
	return err
}

// sharePeers transmits at most 'num' peers over the connection.
// TODO: choose random peers?
func (tcps *TCPServer) sharePeers(conn net.Conn, msgData []byte) error {
	if len(msgData) != 1 {
		return errors.New("invalid number of peers")
	}
	num := msgData[0]
	var addrs []NetAddress
	for addr := range tcps.addressbook {
		if num == 0 {
			break
		}
		addrs = append(addrs, addr)
		num--
	}
	_, err := conn.Write(Marshal(addrs))
	return err
}

// Ping returns whether a NetAddress is reachable. It accomplishes this by
// initiating a TCP connection and immediately closes it. This is pretty
// unsophisticated. I'll add a Pong later.
func (tcps *TCPServer) Ping(addr NetAddress) bool {
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// learnHostname learns the external IP of the TCPServer.
func (tcps *TCPServer) learnHostname(conn net.Conn) (err error) {
	// send hostname request
	if _, err = conn.Write([]byte{'H', 0}); err != nil {
		return
	}
	// read response
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	// TODO: try to ping ourselves?
	host, portStr, err := net.SplitHostPort(string(buf[:n]))
	if err != nil {
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return
	}
	tcps.myAddr = NetAddress{host, uint16(port)}
	return
}

// requestPeers queries a peer for additional peers, and adds any new peers to
// the address book.
func (tcps *TCPServer) requestPeers(conn net.Conn) (err error) {
	// request 10 peers
	if _, err = conn.Write([]byte{'P', 1, 10}); err != nil {
		return
	}
	// read response
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}
	var addrs []NetAddress
	if err = Unmarshal(buf[:n], &addrs); err != nil {
		return
	}
	// add peers
	// TODO: make sure we don't add ourself
	for _, addr := range addrs {
		if tcps.Ping(addr) {
			tcps.addressbook[addr] = struct{}{}
		}
	}
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
		if addr.Call(tcps.learnHostname) == nil {
			break
		}
	}
	// request peers
	// TODO: maybe iterate until we have enough new peers?
	for addr := range tcps.addressbook {
		addr.Call(tcps.requestPeers)
	}
	// TODO: announce ourselves to new peers
	return
}
