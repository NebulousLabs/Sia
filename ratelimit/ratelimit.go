package ratelimit

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// BM is the global bandwidthManager.
var BM *bandwidthManager

type (
	// RLConn is a helper struct that wraps a net.Conn and implements the
	// net.Conn interface.
	RLConn struct {
		mu         sync.Mutex
		writeChan  chan []byte
		readChan   chan []byte
		resultChan chan connResult
		workSignal chan struct{}
		conn       net.Conn
	}
	// connResult contains the return values of a Read or Write.
	connResult struct {
		n   int
		err error
	}
	// bandwidthManager is a singleton that coordinates the RLConnections in
	// the background to guarantee a fair bandwidth distribution over all the
	// connections.
	bandwidthManager struct {
		mu               sync.Mutex
		conns            []*RLConn
		atomicWritePPS   int64
		atomicReadPPS    int64
		atomicPacketSize int64
		threadRunning    bool
	}
)

// Init initializes the bandwidth manager. The first call to Init initializes the bandwidthManager object. Subsequent calls to Init will change the global limits.
func Init(writePPS, readPPS, packetSize int64, cancel chan struct{}) {
	// Check if BM already exists
	if BM != nil {
		build.Critical("bandwidth manager is already initialized")
		return
	}
	BM = &bandwidthManager{
		atomicWritePPS:   writePPS,
		atomicReadPPS:    readPPS,
		atomicPacketSize: packetSize,
	}
	go BM.threadedWriteLoop(cancel)
	go BM.threadedReadLoop(cancel)
}

// NewRLConn wraps a net.Conn into a RLConn and adds it to the
// bandwidthManager.
func NewRLConn(conn net.Conn) net.Conn {
	rlc := &RLConn{
		readChan:   make(chan []byte),
		writeChan:  make(chan []byte),
		resultChan: make(chan connResult),
		conn:       conn,
	}
	BM.managedAddConnection(rlc)
	return rlc
}

// threadedRead reads some data from a connection and sends the result through
// the resultChan.
func threadedRead(b []byte, conn *RLConn) {
	var err error
	var n int
	n, err = conn.conn.Read(b)
	conn.resultChan <- connResult{
		n:   n,
		err: err,
	}
}

// threadedWrite writes some data to a connection and sends the result through
// the resultChan.
func threadedWrite(b []byte, conn *RLConn) {
	var err error
	var n int
	n, err = conn.conn.Write(b)
	conn.resultChan <- connResult{
		n:   n,
		err: err,
	}
}

// threadedReadLoop constantly loops over all connections and checks if any
// connection would like to read a packet.
func (bm *bandwidthManager) threadedReadLoop(cancel chan struct{}) {
	i := 0
	workDone := false
	for {
		// Check for shutdown.
		select {
		case <-cancel:
			return
		default:
		}

		// Grab a connection.
		bm.mu.Lock()
		if len(bm.conns) == 0 {
			// There is no connection to grab.
			bm.mu.Unlock()
			continue
		}
		if i >= len(bm.conns) {
			// If no work was done during this iteration we sleep a bit.
			if !workDone {
				time.Sleep(time.Millisecond)
			}
			workDone = false
			// Start at the beginning again.
			i = 0
		}
		conn := bm.conns[i]
		bm.mu.Unlock()

		// Check if there is work to do.
		var b []byte
		select {
		default:
			continue
		case b = <-conn.readChan:
		}

		// There is some work to do. Wait some time before doing it.
		workDone = true
		pps := atomic.LoadInt64(&bm.atomicReadPPS)
		packetSize := atomic.LoadInt64(&bm.atomicPacketSize)
		packetTime := time.Second * time.Duration(len(b)) / time.Duration(packetSize)
		if pps > 0 {
			select {
			case <-cancel:
				return
			case <-time.After(packetTime / time.Duration(pps)):
			}
		}
		go threadedRead(b, conn)
	}
}

// threadedWriteLoop constantly loops over the connections and checks if a
// connection would like to write a packet.
func (bm *bandwidthManager) threadedWriteLoop(cancel chan struct{}) {
	i := 0
	workDone := false
	for {
		// Check for shutdown.
		select {
		case <-cancel:
			return
		default:
		}

		// Grab a connection.
		bm.mu.Lock()
		if len(bm.conns) == 0 {
			// There is no connection to grab.
			bm.mu.Unlock()
			continue
		}
		if i >= len(bm.conns) {
			// If no work was done during this iteration we sleep a bit.
			if !workDone {
				time.Sleep(time.Millisecond)
			}
			workDone = false
			// Start at the beginning again.
			i = 0
		}
		conn := bm.conns[i]
		bm.mu.Unlock()

		// Check if there is work to do.
		var b []byte
		select {
		default:
			continue
		case b = <-conn.writeChan:
		}

		// There is some work to do. Wait some time before doing it.
		workDone = true
		pps := atomic.LoadInt64(&bm.atomicWritePPS)
		packetSize := atomic.LoadInt64(&bm.atomicPacketSize)
		packetTime := time.Second * time.Duration(len(b)) / time.Duration(packetSize)
		if pps > 0 {
			select {
			case <-cancel:
				return
			case <-time.After(packetTime / time.Duration(pps)):
			}
		}
		go threadedWrite(b, conn)
	}
}

// managedAddConnection adds a new RLConnection to the manager.
func (bm *bandwidthManager) managedAddConnection(conn *RLConn) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.conns = append(bm.conns, conn)
}

// managedRemoveConnection removes a RLConnection from the manager. Should be
// called implicitly by conn.Close
func (bm *bandwidthManager) managedRemoveConnection(conn *RLConn) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for i, c := range bm.conns {
		if c == conn {
			bm.conns = append(bm.conns[:i], bm.conns[i+1:]...)
		}
	}
}

// Close calls the underlying connection's Close method.
func (rlc *RLConn) Close() error {
	BM.managedRemoveConnection(rlc)
	return rlc.conn.Close()
}

// LocalAddr calls the underlying connection's LocalAddr method.
func (rlc *RLConn) LocalAddr() net.Addr {
	return rlc.conn.LocalAddr()
}

// Read reads from the underlying connection without exceeding the rate limit.
func (rlc *RLConn) Read(b []byte) (n int, err error) {
	packetSize := atomic.LoadInt64(&BM.atomicPacketSize)
	for len(b) > 0 {
		// Prepare work
		var data []byte
		if packetSize > 0 && int64(len(b)) > packetSize {
			data = b[:packetSize]
			b = b[packetSize:]
		} else {
			data = b
			b = b[:0]
		}

		// Send work
		rlc.mu.Lock()
		rlc.readChan <- data
		result := <-rlc.resultChan
		rlc.mu.Unlock()

		// Check result
		n += result.n
		if result.err != nil {
			return 0, result.err
		}
	}
	return
}

// RemoteAddr calls the underlying connection's RemoteAddr method.
func (rlc *RLConn) RemoteAddr() net.Addr {
	return rlc.conn.RemoteAddr()
}

// SetDeadline calls the underlying connection's SetDeadline method.
func (rlc *RLConn) SetDeadline(t time.Time) error {
	return rlc.conn.SetDeadline(t)
}

// SetReadDeadline calls the underlying connection's SetReadDeadline method.
func (rlc *RLConn) SetReadDeadline(t time.Time) error {
	return rlc.conn.SetReadDeadline(t)
}

// SetWriteDeadline calls the underlying connection's SetWriteDeadline method.
func (rlc *RLConn) SetWriteDeadline(t time.Time) error {
	return rlc.conn.SetWriteDeadline(t)
}

// Write writes data to the underlying connection without exceeding the rate
// limit.
func (rlc *RLConn) Write(b []byte) (n int, err error) {
	packetSize := atomic.LoadInt64(&BM.atomicPacketSize)
	for len(b) > 0 {
		// Prepare work
		var data []byte
		if packetSize > 0 && int64(len(b)) > packetSize {
			data = b[:packetSize]
			b = b[packetSize:]
		} else {
			data = b
			b = b[:0]
		}

		// Send work
		rlc.mu.Lock()
		rlc.writeChan <- data
		result := <-rlc.resultChan
		rlc.mu.Unlock()

		// Check result
		n += result.n
		if result.err != nil {
			return 0, result.err
		}
	}
	return
}
