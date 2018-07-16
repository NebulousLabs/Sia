package smux

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	siasync "gitlab.com/NebulousLabs/Sia/sync" // TODO: Replace with gitlab.com/NebulousLabs/trymutex
	"github.com/pkg/errors"
)

const (
	defaultAcceptBacklog = 1024
)

var (
	errBrokenPipe      = errors.New("broken pipe")
	errGoAway          = errors.New("stream id overflows, should start a new connection")
	errInvalidProtocol = errors.New("invalid protocol version")
	errLargeFrame      = errors.New("frame is too large to send")
)

// Session defines a multiplexed connection for streams
type Session struct {
	conn        net.Conn
	dataWasRead int32            // used to determine if KeepAlive has failed
	sendMu      siasync.TryMutex // ensures only one thread sends at a time

	config           *Config
	nextStreamID     uint32 // next stream identifier
	nextStreamIDLock sync.Mutex

	bucket       int32         // token bucket
	bucketNotify chan struct{} // used for waiting for tokens

	streams    map[uint32]*Stream // all streams in this session
	streamLock sync.Mutex         // locks streams

	die       chan struct{} // flag session has died
	dieLock   sync.Mutex
	chAccepts chan *Stream

	goAway int32 // flag id exhausted

	deadline atomic.Value
}

func newSession(config *Config, conn net.Conn, client bool) *Session {
	s := new(Session)
	s.die = make(chan struct{})
	s.conn = conn
	s.config = config
	s.streams = make(map[uint32]*Stream)
	s.chAccepts = make(chan *Stream, defaultAcceptBacklog)
	s.bucket = int32(config.MaxReceiveBuffer)
	s.bucketNotify = make(chan struct{}, 1)

	if client {
		s.nextStreamID = 1
	} else {
		s.nextStreamID = 0
	}

	go s.recvLoop()
	// keepaliveSend and keepaliveTimeout need to be separate threads, because
	// the keepaliveSend can block, and especially if the underlying conn has no
	// deadline or a very long deadline, we may not check the keepaliveTimeout
	// for an extended period of time and potentially even end in a deadlock.
	go s.keepAliveSend()
	go s.keepAliveTimeout()
	return s
}

// OpenStream is used to create a new stream
func (s *Session) OpenStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, errBrokenPipe
	}

	// generate stream id
	s.nextStreamIDLock.Lock()
	if s.goAway > 0 {
		s.nextStreamIDLock.Unlock()
		return nil, errGoAway
	}

	s.nextStreamID += 2
	sid := s.nextStreamID
	if sid == sid%2 { // stream-id overflows
		s.goAway = 1
		s.nextStreamIDLock.Unlock()
		return nil, errGoAway
	}
	s.nextStreamIDLock.Unlock()

	stream := newStream(sid, s.config.MaxFrameSize, s)

	if _, err := s.writeFrame(newFrame(cmdSYN, sid), time.Now().Add(s.config.WriteTimeout)); err != nil {
		return nil, errors.Wrap(err, "writeFrame")
	}

	s.streamLock.Lock()
	s.streams[sid] = stream
	s.streamLock.Unlock()
	return stream, nil
}

// AcceptStream is used to block until the next available stream
// is ready to be accepted.
func (s *Session) AcceptStream() (*Stream, error) {
	var deadline <-chan time.Time
	if d, ok := s.deadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(d.Sub(time.Now()))
		defer timer.Stop()
		deadline = timer.C
	}
	select {
	case stream := <-s.chAccepts:
		return stream, nil
	case <-deadline:
		return nil, errTimeout
	case <-s.die:
		return nil, errBrokenPipe
	}
}

// Close is used to close the session and all streams.
func (s *Session) Close() (err error) {
	s.dieLock.Lock()

	select {
	case <-s.die:
		s.dieLock.Unlock()
		return errBrokenPipe
	default:
		close(s.die)
		s.dieLock.Unlock()
		s.streamLock.Lock()
		for k := range s.streams {
			s.streams[k].sessionClose()
		}
		s.streamLock.Unlock()
		s.notifyBucket()
		return s.conn.Close()
	}
}

// notifyBucket notifies recvLoop that bucket is available
func (s *Session) notifyBucket() {
	select {
	case s.bucketNotify <- struct{}{}:
	default:
	}
}

// IsClosed does a safe check to see if we have shutdown
func (s *Session) IsClosed() bool {
	select {
	case <-s.die:
		return true
	default:
		return false
	}
}

// NumStreams returns the number of currently open streams
func (s *Session) NumStreams() int {
	if s.IsClosed() {
		return 0
	}
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	return len(s.streams)
}

// SetDeadline sets a deadline used by Accept* calls.
// A zero time value disables the deadline.
func (s *Session) SetDeadline(t time.Time) error {
	s.deadline.Store(t)
	return nil
}

// notify the session that a stream has closed
func (s *Session) streamClosed(sid uint32) {
	s.streamLock.Lock()
	if n := s.streams[sid].recycleTokens(); n > 0 { // return remaining tokens to the bucket
		if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
			s.notifyBucket()
		}
	}
	delete(s.streams, sid)
	s.streamLock.Unlock()
}

// returnTokens is called by stream to return token after read
func (s *Session) returnTokens(n int) {
	if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
		s.notifyBucket()
	}
}

// session read a frame from underlying connection
// it's data is pointed to the input buffer
func (s *Session) readFrame(buffer []byte) (f Frame, err error) {
	// Ensure that reading a frame follows the global timeout.
	s.conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
	defer s.conn.SetReadDeadline(time.Time{})

	if _, err := io.ReadFull(s.conn, buffer[:headerSize]); err != nil {
		return f, errors.Wrap(err, "readFrame")
	}

	dec := rawHeader(buffer)
	if dec.Version() != version {
		return f, errInvalidProtocol
	}

	f.ver = dec.Version()
	f.cmd = dec.Cmd()
	f.sid = dec.StreamID()
	if length := dec.Length(); length > 0 {
		if _, err := io.ReadFull(s.conn, buffer[headerSize:headerSize+length]); err != nil {
			return f, errors.Wrap(err, "readFrame")
		}
		f.data = buffer[headerSize : headerSize+length]
	}
	return f, nil
}

// recvLoop keeps on reading from underlying connection if tokens are available
func (s *Session) recvLoop() {
	buffer := make([]byte, (1<<16)+headerSize)
	for {
		for atomic.LoadInt32(&s.bucket) <= 0 && !s.IsClosed() {
			<-s.bucketNotify
		}

		if f, err := s.readFrame(buffer); err == nil {
			atomic.StoreInt32(&s.dataWasRead, 1)

			switch f.cmd {
			case cmdNOP:
			case cmdSYN:
				s.streamLock.Lock()
				if _, ok := s.streams[f.sid]; !ok {
					stream := newStream(f.sid, s.config.MaxFrameSize, s)
					s.streams[f.sid] = stream
					select {
					case s.chAccepts <- stream:
					case <-s.die:
					}
				}
				s.streamLock.Unlock()
			case cmdFIN:
				s.streamLock.Lock()
				if stream, ok := s.streams[f.sid]; ok {
					stream.markRST()
					stream.notifyReadEvent()
				}
				s.streamLock.Unlock()
			case cmdPSH:
				s.streamLock.Lock()
				if stream, ok := s.streams[f.sid]; ok {
					atomic.AddInt32(&s.bucket, -int32(len(f.data)))
					stream.pushBytes(f.data)
					stream.notifyReadEvent()
				}
				s.streamLock.Unlock()
			default:
				s.Close()
				return
			}
		} else {
			s.Close()
			return
		}
	}
}

// keepAliveSend will periodically send a keepalive message to the remote peer.
func (s *Session) keepAliveSend() {
	ticker := time.NewTicker(s.config.KeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.die:
			return
		case <-ticker.C:
			s.writeFrame(newFrame(cmdNOP, 0), time.Now().Add(s.config.WriteTimeout))
			s.notifyBucket() // force a signal to the recvLoop
		}
	}
}

// keepAliveTimeout will periodically check that some sort of message has been
// sent by the remote peer, closing the session if not.
func (s *Session) keepAliveTimeout() {
	ticker := time.NewTicker(s.config.KeepAliveTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-s.die:
			return
		case <-ticker.C:
			if !atomic.CompareAndSwapInt32(&s.dataWasRead, 1, 0) {
				s.Close()
				return
			}
		}
	}
}

// writeFrame writes the frame to the underlying connection
// and returns the number of bytes written if successful
func (s *Session) writeFrame(frame Frame, timeout time.Time) (int, error) {
	// Verify the frame data size.
	if len(frame.data) > 1<<16 {
		return 0, errLargeFrame
	}

	// Ensure that the configured WriteTimeout is the maximum amount of time
	// that we can wait to send a single frame.
	latestTimeout := time.Now().Add(s.config.WriteTimeout)
	if timeout.IsZero() || timeout.After(latestTimeout) {
		timeout = latestTimeout
	}

	// Determine how much time remains in the timeout, wait for up to that long
	// to grab the sendMu.
	currentTime := time.Now()
	if !timeout.After(currentTime) {
		return 0, errTimeout
	}
	remaining := timeout.Sub(currentTime)
	if !s.sendMu.TryLockTimed(remaining) {
		return 0, errTimeout
	}
	defer s.sendMu.Unlock()

	// Check again that the stream has not been killed.
	select {
	case <-s.die:
		return 0, errBrokenPipe
	default:
	}

	// Prepare the write data.
	buf := make([]byte, headerSize+len(frame.data))
	buf[0] = frame.ver
	buf[1] = frame.cmd
	binary.LittleEndian.PutUint16(buf[2:], uint16(len(frame.data)))
	binary.LittleEndian.PutUint32(buf[4:], frame.sid)
	copy(buf[headerSize:], frame.data)

	// Write the data using the provided writeTimeout.
	s.conn.SetWriteDeadline(timeout)
	n, err := s.conn.Write(buf[:headerSize+len(frame.data)])
	s.conn.SetWriteDeadline(time.Time{})
	n -= headerSize
	if n < 0 {
		n = 0
	}
	return n, err
}
