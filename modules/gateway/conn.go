package gateway

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	dialTimeout = time.Second * 10
)

// A conn is a monitored TCP connection. It satisfies the modules.NetConn
// interface.
type conn struct {
	nc net.Conn

	startTime time.Time
	lastRead  time.Time
	lastWrite time.Time

	nRead    uint64
	nWritten uint64
}

// Read implements the io.Reader interface. Successful reads will reset the
// read timeout. If the connection has already timed out, Read will return an
// error without reading anything.
func (c *conn) Read(b []byte) (n int, err error) {
	n, err = c.nc.Read(b)
	c.nRead += uint64(n)
	c.lastRead = time.Now()
	return
}

// Write implements the io.Writer interface. Successful writes will reset the
// write timeout. If the connection has already timed out, Write will return
// an error without writing anything.
func (c *conn) Write(b []byte) (n int, err error) {
	n, err = c.nc.Write(b)
	c.nWritten += uint64(n)
	c.lastWrite = time.Now()
	return
}

func (c *conn) Close() error {
	return c.nc.Close()
}

func (c *conn) ReadObject(obj interface{}, maxLen uint64) error {
	return encoding.ReadObject(c, obj, maxLen)
}

func (c *conn) WriteObject(obj interface{}) error {
	return encoding.WriteObject(c, obj)
}

// Addr returns the NetAddress of the remote end of the connection.
func (c *conn) Addr() modules.NetAddress {
	return modules.NetAddress(c.nc.RemoteAddr().String())
}

// dial wraps the connection returned by net.Dial in a conn.
func dial(addr modules.NetAddress) (*conn, error) {
	nc, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return nil, err
	}
	return &conn{
		nc:        nc,
		startTime: time.Now(),
	}, nil
}

// accept wraps the connection return by net.Accept in a conn.
func accept(l net.Listener) (*conn, error) {
	nc, err := l.Accept()
	if err != nil {
		return nil, err
	}
	return &conn{
		nc:        nc,
		startTime: time.Now(),
	}, nil
}
