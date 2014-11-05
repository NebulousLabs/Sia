package sia

import (
	//"errors"
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
}

func NewTCPServer(port uint16) (tcps *TCPServer, err error) {
	tcpServ, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return
	}
	tcps = &TCPServer{
		Listener:    tcpServ,
		addressbook: make(map[NetAddress]struct{}),
	}
	go tcps.listen()
	return
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
		msgType   []byte = make([]byte, 1)
		msgLenBuf []byte = make([]byte, 4)
		msgData   []byte // length determined by msgLen
	)
	// TODO: make this DRYer?
	if n, err := conn.Read(msgType); err != nil || n != 1 {
		// TODO: log error
		return
	}
	if n, err := conn.Read(msgLenBuf); err != nil || n != 4 {
		// TODO: log error
		return
	}
	msgLen := DecUint64(msgLenBuf)
	if msgLen > maxMsgLen {
		// TODO: log error
		return
	}
	msgData = make([]byte, msgLen)
	if n, err := conn.Read(msgData); err != nil || uint64(n) != msgLen {
		// TODO: log error
		return
	}

	switch msgType[0] {
	// Hostname discovery
	case 'H':
		_, err := conn.Write([]byte(conn.RemoteAddr().String()))
		if err != nil {
			// TODO: log error
			return
		}

	// Peer discovery
	case 'P':
		if msgLen != 1 {
			// TODO: log error
			return
		}
		tcps.sharePeers(conn, msgData[0])

	// Block
	case 'B':
		var b Block
		if err := Unmarshal(msgData, &b); err != nil {
			// TODO: log error
			return
		}
		//state.AcceptBlock(b)
		// TODO: log error

	// Transaction
	case 'T':
		var t Transaction
		if err := Unmarshal(msgData, &t); err != nil {
			// TODO: log error
			return
		}
		//state.AcceptTransaction(&t)
		// TODO: log error

	// Unknown
	default:
		// TODO: log error
	}
	return
}

// sharePeers transmits at most 'num' peers over the connection.
// TODO: choose random peers?
func (tcps *TCPServer) sharePeers(conn net.Conn, num uint8) {
	var addrs []NetAddress
	for addr := range tcps.addressbook {
		if num == 0 {
			break
		}
		addrs = append(addrs, addr)
		num--
	}
	conn.Write(Marshal(addrs))
	// log error?
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
