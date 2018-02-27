package conn

import (
	"net"
	"time"
)

type (
	// RLConnection is a wrapper for the net.Conn object that allows
	// the user to limit the connection throughput.
	RLConnection struct {
		conn            net.Conn  // underlying net.Conn
		readBytes       int64     // number of bytes that have been read from the last packet
		readPPS         int64     // packets that are read per second
		readPacketSize  int64     // size of a single packet being read in bytes
		writtenBytes    int64     // number of bytes that have been written to the latest packet
		writePPS        int64     // packets that are sent per second
		writePacketSize int64     // size of single packet being written in bytes
		lastRead        time.Time // time when last packet was read from
		lastWrite       time.Time // time when last packet was written to
	}
)

// NewRLConn wraps a net.Conn in a RLConnection.
func NewRLConn(conn net.Conn, readPacketSize, readPPS, writePacketSize, writePPS int64) net.Conn {
	rlc := &RLConnection{
		conn:            conn,
		readBytes:       readPacketSize,
		readPPS:         readPPS,
		readPacketSize:  readPacketSize,
		writtenBytes:    writePacketSize,
		writePPS:        writePPS,
		writePacketSize: writePacketSize,
	}
	return rlc
}

// Read calls the underlying connection's Read method.
func (rlc *RLConnection) Read(b []byte) (n int, err error) {
	// If there is no rate limit, we can read everything at once.
	if rlc.readPPS == 0 {
		return rlc.conn.Read(b)
	}

	for len(b) > 0 {
		// Check if we need to sleep
		if rlc.readBytes >= rlc.readPacketSize {
			time.Sleep(time.Second / time.Duration(rlc.readPPS))
			rlc.readBytes = 0
		}
		// Write data
		writableBytes := rlc.readPacketSize - rlc.readBytes
		var written int
		if int64(len(b)) <= writableBytes {
			written, err = rlc.conn.Write(b)
			b = b[:0]
			rlc.readBytes += int64(len(b))
		} else {
			written, err = rlc.conn.Write(b[:writableBytes])
			b = b[writableBytes:]
			rlc.readBytes = rlc.readPacketSize
		}
		n += written
		if err != nil {
			return
		}
		// Remember the last write's timestamp
		rlc.lastRead = time.Now()
	}
	return
}

// Write writes data to the underlying connection without exceeding the rate
// limit.
func (rlc *RLConnection) Write(b []byte) (n int, err error) {
	// If there is no rate limit, we can write everything at once.
	if rlc.writePPS == 0 {
		return rlc.conn.Write(b)
	}

	for len(b) > 0 {
		// Check if we need to sleep
		if rlc.writtenBytes >= rlc.writePacketSize {
			time.Sleep(time.Second / time.Duration(rlc.writePPS))
			rlc.writtenBytes = 0
		}
		// Write data
		writableBytes := rlc.writePacketSize - rlc.writtenBytes
		var written int
		if int64(len(b)) <= writableBytes {
			written, err = rlc.conn.Write(b)
			b = b[:0]
			rlc.writtenBytes += int64(len(b))
		} else {
			written, err = rlc.conn.Write(b[:writableBytes])
			b = b[writableBytes:]
			rlc.writtenBytes = rlc.writePacketSize
		}
		n += written
		if err != nil {
			return
		}
		// Remember the last write's timestamp
		rlc.lastWrite = time.Now()
	}
	return
}

// Close calls the underlying connection's Close method.
func (rlc *RLConnection) Close() error {
	return rlc.conn.Close()
}

// LocalAddr calls the underlying connection's LocalAddr method.
func (rlc *RLConnection) LocalAddr() net.Addr {
	return rlc.conn.LocalAddr()
}

// RemoteAddr calls the underlying connection's RemoteAddr method.
func (rlc *RLConnection) RemoteAddr() net.Addr {
	return rlc.conn.RemoteAddr()
}

// SetDeadline calls the underlying connection's SetDeadline method.
func (rlc *RLConnection) SetDeadline(t time.Time) error {
	return rlc.conn.SetDeadline(t)
}

// SetReadDeadline calls the underlying connection's SetReadDeadline method.
func (rlc *RLConnection) SetReadDeadline(t time.Time) error {
	return rlc.conn.SetReadDeadline(t)
}

// SetWriteDeadline calls the underlying connection's SetWriteDeadline method.
func (rlc *RLConnection) SetWriteDeadline(t time.Time) error {
	return rlc.conn.SetWriteDeadline(t)
}
