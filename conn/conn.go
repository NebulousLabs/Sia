package conn

import (
	"net"
	"time"
)

type (
	// RLConnection is a wrapper for the net.Conn object that allows
	// the user to limit the connection throughput.
	RLConnection struct {
		conn             net.Conn  // underlying net.Conn
		packetSize       uint64    // size of single packet in bytes
		packetsPerSecond uint64    // packets that are sent per second
		writtenBytes     uint64    // number of bytes that have been written to the latest packet
		lastWrite        time.Time // time when last packet was started
	}
)

// Dial creates a RateLimitedDialer.
func Dial(dialer *net.Dialer, network string, address string) (*RLConnection, error) {
	conn, err := dialer.Dial(network, address)
	if err != nil {
		return nil, err
	}
	rlc := &RLConnection{
		conn: conn,
	}
	return rlc, nil
}

// Read calls the underlying connection's Read method.
func (rlc *RLConnection) Read(b []byte) (n int, err error) {
	return rlc.conn.Read(b)
}

// SetLimit sets the rate limit of the connection.
func (rlc *RLConnection) SetLimit(packetSize uint64, packetsPerSecond uint64) {
	rlc.packetSize = packetSize
	rlc.packetsPerSecond = packetsPerSecond
	rlc.lastWrite = time.Now()
	rlc.writtenBytes = rlc.packetSize
}

// Write writes data to the underlying connection without exceeding the rate
// limit.
func (rlc *RLConnection) Write(b []byte) (n int, err error) {
	// If there is no rate limit, we can write everything at once.
	if rlc.packetsPerSecond == 0 {
		return rlc.conn.Write(b)
	}

	// Start a feeder thread that feeds us the data at the right speed.
	packets := make(chan []byte)
	go func() {
		defer close(packets)
		if rlc.writtenBytes < rlc.packetSize {
			// We have previously started writing a packet that has some space
			// left. We can write right away.
			remainingBytes := rlc.packetSize - rlc.writtenBytes
			rlc.writtenBytes = rlc.packetSize
			if uint64(len(b)) <= remainingBytes {
				packets <- b
				return
			}
			packets <- b[:remainingBytes]
			b = b[remainingBytes:]
		}
		// Write the remaining data packet by packet.
		timer := time.NewTimer(time.Until(rlc.lastWrite.Add(time.Second)))
		for len(b) > 0 {
			// Wait long enough
			select {
			case <-timer.C:
			}
			// Pass the data on to the writing thread
			if uint64(len(b)) <= rlc.packetSize {
				packets <- b
				b = b[:0]
				rlc.writtenBytes = uint64(len(b))
			} else {
				packets <- b[:rlc.packetSize]
				b = b[rlc.packetSize:]
				rlc.writtenBytes = rlc.packetSize
			}
			// Remember the last write's timestamp
			rlc.lastWrite = time.Now()
			timer.Reset(time.Second)
		}
	}()

	// Write data one packet at a time.
	for packet := range packets {
		written, err := rlc.conn.Write(packet)
		n += written
		if err != nil {
			return 0, err
		}
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
