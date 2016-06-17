package gateway

import (
	"net"

	muxadov1 "github.com/NebulousLabs/muxado"    // used pre-1.0
	muxadov2 "github.com/inconshreveable/muxado" // used post-1.0
)

// A streamSession is a multiplexed transport that can accept or initiate
// streams.
type streamSession interface {
	Accept() (net.Conn, error)
	Open() (net.Conn, error)
	Close() error
}

// muxado's Session methods do not return a net.Conn, but rather a
// muxado.Stream, necessitating an adaptor.
type muxadoAdaptor struct {
	sess muxadov1.Session
}

func (m muxadoAdaptor) Accept() (net.Conn, error) { return m.sess.Accept() }
func (m muxadoAdaptor) Open() (net.Conn, error)   { return m.sess.Open() }
func (m muxadoAdaptor) Close() error              { return m.sess.Close() }

func newMuxadoV1Server(conn net.Conn) muxadoAdaptor {
	return muxadoAdaptor{muxadov1.Server(conn)}
}

func newMuxadoV1Client(conn net.Conn) muxadoAdaptor {
	return muxadoAdaptor{muxadov1.Client(conn)}
}

func newMuxadoV2Server(conn net.Conn) muxadov2.Session {
	return muxadov2.Server(conn, nil)
}

func newMuxadoV2Client(conn net.Conn) muxadov2.Session {
	return muxadov2.Client(conn, nil)
}
