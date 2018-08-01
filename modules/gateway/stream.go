package gateway

import (
	"net"

	"github.com/NebulousLabs/Sia/build"
	"github.com/xtaci/smux"
)

// A streamSession is a multiplexed transport that can accept or initiate
// streams.
type streamSession interface {
	Accept() (net.Conn, error)
	Open() (net.Conn, error)
	Close() error
}

// newClientStream returns a new smux client.
func newClientStream(conn net.Conn, version string) streamSession {
	return newSmuxClient(conn)
}

// newServerStream returns a new smux server.
func newServerStream(conn net.Conn, version string) streamSession {
	return newSmuxServer(conn)
}

// smuxSession adapts the methods of smux.Session to conform to the
// streamSession interface.
type smuxSession struct {
	sess *smux.Session
}

func (s smuxSession) Accept() (net.Conn, error) { return s.sess.AcceptStream() }
func (s smuxSession) Open() (net.Conn, error)   { return s.sess.OpenStream() }
func (s smuxSession) Close() error              { return s.sess.Close() }

func newSmuxServer(conn net.Conn) streamSession {
	sess, err := smux.Server(conn, nil) // default config means no error is possible
	if err != nil {
		build.Critical("smux should not fail with default config:", err)
	}
	return smuxSession{sess}
}

func newSmuxClient(conn net.Conn) streamSession {
	sess, err := smux.Client(conn, nil) // default config means no error is possible
	if err != nil {
		build.Critical("smux should not fail with default config:", err)
	}
	return smuxSession{sess}
}
