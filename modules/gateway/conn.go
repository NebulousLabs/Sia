package gateway

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

// A Conn is a monitored network connection.
type Conn struct {
	nc net.Conn

	startTime time.Time
	lastRead  time.Time
	lastWrite time.Time

	nRead    uint64
	nWritten uint64
}

// Read implements the io.Reader interface. Successful reads will reset the
// read timeout. If the Connection has already timed out, Read will return an
// error without reading anything.
func (c *Conn) Read(b []byte) (n int, err error) {
	n, err = c.nc.Read(b)
	c.nRead += uint64(n)
	c.lastRead = time.Now()
	return
}

// Write implements the io.Writer interface. Successful writes will reset the
// write timeout. If the Connection has already timed out, Write will return
// an error without writing anything.
func (c *Conn) Write(b []byte) (n int, err error) {
	n, err = c.nc.Write(b)
	c.nWritten += uint64(n)
	c.lastWrite = time.Now()
	return
}

func (c *Conn) Close() error {
	return c.nc.Close()
}

func (c *Conn) ReadObject(obj interface{}, maxLen uint64) error {
	return encoding.ReadObject(c, obj, maxLen)
}

func (c *Conn) WriteObject(obj interface{}) error {
	return encoding.WriteObject(c, obj)
}

// Addr returns the Address of the remote end of the connection.
func (c *Conn) Addr() Address {
	return Address(c.nc.RemoteAddr().String())
}

func newConn(nc net.Conn) *Conn {
	return &Conn{
		nc:        nc,
		startTime: time.Now(),
	}
}
