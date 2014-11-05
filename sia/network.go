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

func (na *NetAddress) String() string {
	return net.JoinHostPort(addr.Host, strconv.Itoa(int(addr.Port)))
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
		msgType   = make([]byte, 1)
		msgLenBuf = make([]byte, 4)
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

	// Block
	case 'B':
		var b Block
		if err := Unmarshal(msgData, &b); err != nil {
			// TODO: log error
			return
		}
		//state.ProcessBlock(b)?

	// Transaction
	case 'T':
		var t Transaction
		if err := Unmarshal(msgData, &t); err != nil {
			// TODO: log error
			return
		}
		//state.ProcessTransaction(t)?

	// Unknown
	default:
		// TODO: log error
	}
	return
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

// send initiates a TCP connection and writes a message to it.
// TODO: add timeout
func (tcps *TCPServer) send(msg []byte, addr NetAddress) (err error) {
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		return
	}
	_, err = conn.Write(msg)
	return
}

// learnHostname learns the external IP of the TCPServer.
func (tcps *TCPServer) learnHostname(addr NetAddress) (err error) {
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		return
	}
	defer conn.Close()
	// send hostname request
	if _, err = conn.Write([]byte{'H', 0}); err != nil {
		return
	}
	// read response
	buf = make([]byte, 128)
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

// Bootstrap calls Request on a predefined set of peers in order to build up an
// initial peer list. It returns the number of peers added.
func (tcps *TCPServer) Bootstrap() int {
	n := len(tcps.addressbook)
	// for _, host := range BootstrapPeers {

	// }
	return len(tcps.addressbook) - n
}
