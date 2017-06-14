package gateway

import (
	"net"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/muxado"
	"github.com/xtaci/smux"
)

// A streamSession is a multiplexed transport that can accept or initiate
// streams.
type streamSession interface {
	Accept() (net.Conn, error)
	Open() (net.Conn, error)
	Close() error
}

// returns a new client stream, with a protocol that works on top of the TCP connection.
// using smux for version >= 1.3.0, and using muxado otherwise.
func newClientStream(conn net.Conn, version string) streamSession {
	if build.VersionCmp(version, sessionUpgradeVersion) >= 0 {
		return newSmuxClient(conn)
	}
	return newMuxadoClient(conn)
}

// returns a new server stream, with a protocol that works on top of the TCP connection.
// using smux for version >= 1.3.0, and using muxado otherwise.
func newServerStream(conn net.Conn, version string) streamSession {
	if build.VersionCmp(version, sessionUpgradeVersion) >= 0 {
		return newSmuxServer(conn)
	}
	return newMuxadoServer(conn)
}

// muxado's Session methods do not return a net.Conn, but rather a
// muxado.Stream, necessitating an adaptor.
type muxadoSession struct {
	sess muxado.Session
}

func (m muxadoSession) Accept() (net.Conn, error) { return m.sess.Accept() }
func (m muxadoSession) Open() (net.Conn, error)   { return m.sess.Open() }
func (m muxadoSession) Close() error              { return m.sess.Close() }

func newMuxadoServer(conn net.Conn) streamSession {
	return muxadoSession{muxado.Server(conn)}
}

func newMuxadoClient(conn net.Conn) streamSession {
	return muxadoSession{muxado.Client(conn)}
}

// smux's Session methods do not return a net.Conn, but rather a
// smux.Stream, necessitating an adaptor.
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
