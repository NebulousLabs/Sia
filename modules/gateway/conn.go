package gateway

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	// time to wait for peer to "pick up"
	dialTimeout = 10 * time.Second

	// after each read or write, the connection timeout is reset
	timeout = 10 * time.Second
)

var (
	ErrTimeout = errors.New("timeout")
)

// conn is a barebones type that implements the modules.NetConn interface.
type conn struct {
	net.Conn
}

// Read implements the io.Reader interface. Timeout errors result in
// ErrTimeout.
func (c *conn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = ErrTimeout
	}
	return
}

// Write implements the io.Writer interface. Timeout errors result in
// ErrTimeout.
func (c *conn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = ErrTimeout
	}
	return
}

// ReadObject implements the encoding.Reader interface.
func (c *conn) ReadObject(obj interface{}, maxLen uint64) error {
	return encoding.ReadObject(c, obj, maxLen)
}

// WriteObject implements the encoding.Writer interface.
func (c *conn) WriteObject(obj interface{}) error {
	return encoding.WriteObject(c, obj)
}

// Addr returns the NetAddress of the remote end of the connection.
func (c *conn) Addr() modules.NetAddress {
	return modules.NetAddress(c.RemoteAddr().String())
}

// netConn wraps a net.Conn to implement the methods of modules.NetConn.
func newConn(c net.Conn) *conn {
	return &conn{c}
}

// dial wraps the connection returned by net.Dial in a conn.
func dial(addr modules.NetAddress) (*conn, error) {
	nc, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return nil, err
	}
	return newConn(nc), nil
}

// accept wraps the connection return by listener.Accept in a conn.
func accept(l net.Listener) (*conn, error) {
	nc, err := l.Accept()
	if err != nil {
		return nil, err
	}
	return newConn(nc), nil
}
