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

// A conn is a monitored TCP connection. It satisfies the modules.NetConn
// interface.
type conn struct {
	nc net.Conn
}

// Read implements the io.Reader interface. Successful reads will reset the
// read timeout. If the connection has already timed out, Read will return an
// error without reading anything.
func (c *conn) Read(b []byte) (n int, err error) {
	n, err = c.nc.Read(b)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = ErrTimeout
	}
	c.nc.SetDeadline(time.Now().Add(timeout))
	return
}

// Write implements the io.Writer interface. Successful writes will reset the
// write timeout. If the connection has already timed out, Write will return
// an error without writing anything.
func (c *conn) Write(b []byte) (n int, err error) {
	n, err = c.nc.Write(b)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		err = ErrTimeout
	}
	c.nc.SetDeadline(time.Now().Add(timeout))
	return
}

func (c *conn) Close() error {
	return c.nc.Close()
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
	return modules.NetAddress(c.nc.RemoteAddr().String())
}

// newConn creates a new conn from a net.Conn.
func newConn(nc net.Conn) *conn {
	return &conn{
		nc: nc,
	}
}

// dial wraps the connection returned by net.Dial in a conn.
func dial(addr modules.NetAddress) (*conn, error) {
	nc, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return nil, err
	}
	return newConn(nc), nil
}

// accept wraps the connection return by net.Accept in a conn.
func accept(l net.Listener) (*conn, error) {
	nc, err := l.Accept()
	if err != nil {
		return nil, err
	}
	return newConn(nc), nil
}
