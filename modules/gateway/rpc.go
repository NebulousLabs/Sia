package gateway

import (
	"errors"
	"net"
	"sync"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

type rpcID [8]byte

func (id rpcID) String() string {
	for i := range id {
		if id[i] == 0 {
			id[i] = ' '
		}
	}
	return string(id[:])
}

// handlerName truncates a string to 8 bytes. If len(name) < 8, the remaining
// bytes are 0. A handlerName is specified at the beginning of each network
// call, indicating which function should handle the connection.
func handlerName(name string) (id rpcID) {
	copy(id[:], name)
	return
}

// RPC calls an RPC on the given address. RPC cannot be called on an address
// that the Gateway is not connected to.
func (g *Gateway) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	g.log.Printf("INFO: calling RPC \"%v\" on %v\n", name, addr)
	peer, ok := g.peers[addr]
	if !ok {
		return errors.New("can't call RPC on unconnected peer " + string(addr))
	}

	conn, err := peer.sess.Open()
	if err != nil {
		return err
	}
	defer conn.Close()

	// write header
	if err := encoding.WriteObject(conn, handlerName(name)); err != nil {
		return err
	}
	// call fn
	err = fn(conn)
	if err != nil {
		// TODO: use sync/atomic
		peer.strikes++
	}
	return err
}

// readerRPC returns a closure that can be passed to RPC to read a
// single value.
func readerRPC(obj interface{}, maxLen uint64) modules.RPCFunc {
	return func(conn net.Conn) error {
		return encoding.ReadObject(conn, obj, maxLen)
	}
}

// writerRPC returns a closure that can be passed to RPC to write a
// single value.
func writerRPC(obj interface{}) modules.RPCFunc {
	return func(conn net.Conn) error {
		return encoding.WriteObject(conn, obj)
	}
}

// RegisterRPC registers a function as an RPC handler for a given identifier.
// To call an RPC, use gateway.RPC, supplying the same identifier given to
// RegisterRPC. Identifiers should always use PascalCase.
func (g *Gateway) RegisterRPC(name string, fn modules.RPCFunc) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	g.handlerMap[handlerName(name)] = fn
}

// listenPeer listens for new streams on a peer connection and serves them via
// threadedHandleConn.
func (g *Gateway) listenPeer(p *peer) {
	for {
		conn, err := p.sess.Accept()
		if err != nil {
			return
		}

		// it is the handler's responsibility to close the connection
		go g.threadedHandleConn(conn)
	}
}

// threadedHandleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (g *Gateway) threadedHandleConn(conn net.Conn) {
	defer conn.Close()
	var id rpcID
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		g.log.Printf("WARN: could not read RPC identifier from incoming conn %v: %v\n", conn.RemoteAddr(), err)
		return
	}
	// call registered handler for this ID
	lockid := g.mu.RLock()
	fn, ok := g.handlerMap[id]
	g.mu.RUnlock(lockid)
	if !ok {
		g.log.Printf("WARN: incoming conn %v requested unknown RPC \"%s\"", conn.RemoteAddr(), id[:])
		return
	}

	g.log.Printf("INFO: handling RPC \"%v\" from %v\n", id, conn.RemoteAddr())
	if err := fn(conn); err != nil {
		g.log.Printf("WARN: incoming RPC \"%v\" failed: %v\n", id, err)
	}
}

// threadedBroadcast calls an RPC on all of the peers in the Gateway's peer
// list. The calls are run in parallel.
func (g *Gateway) threadedBroadcast(name string, fn modules.RPCFunc) {
	g.log.Printf("INFO: broadcasting RPC \"%v\" to %v peers\n", name, len(g.peers))
	var wg sync.WaitGroup
	wg.Add(len(g.peers))
	id := g.mu.RLock()
	for addr := range g.peers {
		// contact each peer in a separate thread
		go func(addr modules.NetAddress) {
			err := g.RPC(addr, name, fn)
			if err != nil {
				g.log.Printf("WARN: broadcast: calling RPC \"%v\" on peer %v returned error: %v\n", name, addr, err)
			}
			wg.Done()
		}(addr)
	}
	g.mu.RUnlock(id)
	wg.Wait()
}
