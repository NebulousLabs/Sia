package network

import (
	"errors"
	"net"
	"reflect"
	"strconv"
	"time"

	"github.com/NebulousLabs/Andromeda/encoding"
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

func ReadPrefix(conn net.Conn) ([]byte, error) {
	prefix := make([]byte, 4)
	if n, err := conn.Read(prefix); err != nil || n != len(prefix) {
		return nil, errors.New("could not read length prefix")
	}
	msgLen := int(encoding.DecUint64(prefix))
	if msgLen > maxMsgLen {
		return nil, errors.New("message too long")
	}
	// read msgLen bytes
	var data []byte
	buf := make([]byte, 1024)
	for total := 0; total < msgLen; {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		data = append(data, buf[:n]...)
		total += n
	}
	if len(data) != msgLen {
		return nil, errors.New("message length mismatch")
	}
	return data, nil
}

func ReadObject(conn net.Conn, obj interface{}) error {
	data, err := ReadPrefix(conn)
	if err != nil {
		return err
	}
	return encoding.Unmarshal(data, obj)
}

func WritePrefix(conn net.Conn, data []byte) (int, error) {
	encLen := encoding.EncUint64(uint64(len(data)))
	return conn.Write(append(encLen[:4], data...))
}

func WriteObject(conn net.Conn, obj interface{}) (int, error) {
	return WritePrefix(conn, encoding.Marshal(obj))
}

// RPC performs a Remote Procedure Call by sending the procedure name and
// encoded argument, and decoding the response into the supplied object.
// 'resp' must be a pointer. If arg is nil, no object is sent. If 'resp' is
// nil, no response is read.
func (na *NetAddress) RPC(t byte, arg, resp interface{}) error {
	return na.Call(func(conn net.Conn) error {
		conn.Write([]byte{t})
		if arg != nil {
			if _, err := WriteObject(conn, arg); err != nil {
				return err
			}
		}
		if resp != nil {
			return ReadObject(conn, resp)
		}
		return nil
	})
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
	}
	// default handlers
	tcps.handlerMap = map[byte]func(net.Conn, []byte) error{
		'H': sendHostname,
		'P': tcps.sharePeers,
		'A': tcps.addPeer,
	}

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
		if addr.Call(tcps.learnHostname) == nil {
			break
		}
	}

	// request peers
	// TODO: maybe iterate until we have enough new peers?
	tcps.Broadcast(tcps.requestPeers)

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
			conn.Write([]byte{'A'})
			_, err := WriteObject(conn, obj)
			return err
		})
	}
}

func (tcps *TCPServer) Register(t byte, fn interface{}) {
	// all handlers are function with at least one in and one error out
	val, typ := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if typ.Kind() != reflect.Func || typ.NumIn() < 1 ||
		typ.NumOut() != 1 || typ.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		panic("registered function has wrong type signature")
	}

	switch {
	// func(net.Conn, []byte) error
	case typ.NumIn() == 2 && typ.In(0) == reflect.TypeOf((*net.Conn)(nil)).Elem() && typ.In(1) == reflect.TypeOf([]byte{}):
		tcps.handlerMap[t] = fn.(func(net.Conn, []byte) error)
	// func(Type, *Type) error
	case typ.NumIn() == 2 && typ.In(0).Kind() != reflect.Ptr && typ.In(1).Kind() == reflect.Ptr:
		tcps.registerRPC(t, val, typ)
	// func(Type) error
	case typ.NumIn() == 1 && typ.In(0).Kind() != reflect.Ptr:
		tcps.registerArg(t, val, typ)
	// func(*Type) error
	case typ.NumIn() == 1 && typ.In(0).Kind() == reflect.Ptr:
		tcps.registerResp(t, val, typ)
	default:
		panic("registered function has wrong type signature")
	}
}

// registerRPC is for handlers that return a value. The input is decoded and
// passed to fn, which stores its result in a pointer argument. This argument
// is then written back to the caller. fn must have the type signature:
//     func(Type, *Type) error
func (tcps *TCPServer) registerRPC(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(conn net.Conn, b []byte) error {
		// create object to decode into
		arg := reflect.New(typ.In(0))
		if err := encoding.Unmarshal(b, arg.Interface()); err != nil {
			return err
		}
		// call fn on object
		resp := reflect.New(typ.In(1))
		if err := fn.Call([]reflect.Value{arg.Elem(), resp})[0].Interface(); err != nil {
			return err.(error)
		}
		// write response
		_, err := WriteObject(conn, resp.Elem().Interface())
		return err
	}
}

// registerArg is for RPCs that do not return a value.
func (tcps *TCPServer) registerArg(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(_ net.Conn, b []byte) error {
		// create object to decode into
		arg := reflect.New(typ.In(0))
		if err := encoding.Unmarshal(b, arg.Interface()); err != nil {
			return err
		}
		// call fn on object
		if err := fn.Call([]reflect.Value{arg.Elem()})[0].Interface(); err != nil {
			return err.(error)
		}
		return nil
	}
}

// registerResp is for RPCs that do not take a value.
func (tcps *TCPServer) registerResp(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(conn net.Conn, _ []byte) error {
		// create object to hold response
		resp := reflect.New(typ.In(0))
		// call fn
		if err := fn.Call([]reflect.Value{resp})[0].Interface(); err != nil {
			return err.(error)
		}
		// write response
		_, err := WriteObject(conn, resp.Elem().Interface())
		return err
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

// sendHostname replies to the send with the sender's external IP.
func sendHostname(conn net.Conn, _ []byte) error {
	_, err := WritePrefix(conn, []byte(conn.RemoteAddr().String()))
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
	_, err := WritePrefix(conn, encoding.Marshal(addrs))
	return err
}

// addPeer adds the connecting peer to its address book
func (tcps *TCPServer) addPeer(_ net.Conn, data []byte) (err error) {
	var addr NetAddress
	if err = encoding.Unmarshal(data, &addr); err != nil {
		return
	}
	tcps.addressbook[addr] = struct{}{}
	return
}

// learnHostname learns the external IP of the TCPServer.
func (tcps *TCPServer) learnHostname(conn net.Conn) (err error) {
	// send hostname request
	if _, err = conn.Write([]byte{'H', 0, 0, 0, 0}); err != nil {
		return
	}
	// read response
	data, err := ReadPrefix(conn)
	if err != nil {
		return
	}
	// TODO: try to ping ourselves?
	host, _, err := net.SplitHostPort(string(data))
	if err != nil {
		return
	}
	tcps.myAddr.Host = host
	return
}

// requestPeers queries a peer for additional peers, and adds any new peers to
// the address book.
func (tcps *TCPServer) requestPeers(conn net.Conn) (err error) {
	// request 10 peers
	if _, err = conn.Write([]byte{'P', 1, 0, 0, 0, 10}); err != nil {
		return
	}
	// read response
	data, err := ReadPrefix(conn)
	if err != nil {
		return
	}
	var addrs []NetAddress
	if err = encoding.Unmarshal(data, &addrs); err != nil {
		return
	}
	// add peers
	for _, addr := range addrs {
		if addr != tcps.myAddr && tcps.Ping(addr) {
			tcps.addressbook[addr] = struct{}{}
		}
	}
	return
}
