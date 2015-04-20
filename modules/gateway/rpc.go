package gateway

import (
	"errors"
	"net"
	"sync"

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

func rpc(conn modules.NetConn, name string, fn modules.RPCFunc) error {
	defer conn.Close()
	// write header
	if err := conn.WriteObject(handlerName(name)); err != nil {
		return err
	}
	// call fn
	return fn(conn)
}

// RPC establishes a TCP connection to the NetAddress, writes the RPC
// identifier, and then hands off the connection to fn. When fn returns, the
// connection is closed.
func (g *Gateway) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	conn, err := dial(addr)
	if err != nil {
		return err
	}
	return rpc(conn, name, fn)
}

// streamRPC is used for calling an RPC on a connected peer. If the RPC returns
// an error, the peer will be penalized.
func (g *Gateway) streamRPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	peer, ok := g.peers[addr]
	if !ok {
		return errors.New("not connected to peer: " + string(addr))
	}

	conn, err := peer.sess.Open()
	if err != nil {
		return err
	}
	err = rpc(modules.NewNetConn(conn), name, fn)
	if err != nil {
		id := g.mu.Lock()
		g.addStrike(addr)
		g.mu.Unlock(id)
	}
	return err
}

// RegisterRPC registers a function as an RPC handler for a given identifier.
// To call an RPC, use gateway.RPC, supplying the same identifier given to
// RegisterRPC. Identifiers should always use PascalCase.
func (g *Gateway) RegisterRPC(name string, fn modules.RPCFunc) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	g.handlerMap[handlerName(name)] = fn
}

// readerRPC returns a closure that can be passed to RPC to read a
// single value.
func readerRPC(obj interface{}, maxLen uint64) modules.RPCFunc {
	return func(conn modules.NetConn) error {
		return conn.ReadObject(obj, maxLen)
	}
}

// writerRPC returns a closure that can be passed to RPC to write a
// single value.
func writerRPC(obj interface{}) modules.RPCFunc {
	return func(conn modules.NetConn) error {
		return conn.WriteObject(obj)
	}
}

// listen listens for new connections, wraps them in a monitor object, and
// serves them via threadedHandleConn. One 'master' listen process handles new
// incoming RPCs, and one process per peer handles new streams opened by that
// peer.
func (g *Gateway) listen(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}

		// it is the handler's responsibility to close the connection
		go g.threadedHandleConn(modules.NewNetConn(conn))
	}
}

// threadedHandleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (g *Gateway) threadedHandleConn(conn modules.NetConn) {
	defer conn.Close()
	var id rpcID
	if err := conn.ReadObject(&id, 8); err != nil {
		g.log.Printf("WARN: could not read RPC identifier from incoming conn %v: %v\n", conn.Addr(), err)
		return
	}
	// call registered handler for this ID
	id := g.mu.RLock()
	fn, ok := g.handlerMap[id]
	g.mu.RUnlock(id)
	if !ok {
		g.log.Printf("WARN: incoming conn %v requested unknown RPC \"%s\"", conn.Addr(), id[:])
		return
	}

	g.log.Printf("INFO: handling RPC \"%v\" from %v\n", id, conn.Addr())
	if err := fn(conn); err != nil {
		g.log.Printf("WARN: incoming RPC \"%v\" failed: %v\n", id, err)
	}

	return
}

// threadedBroadcast calls an RPC on all of the peers in the Gateway's peer
// list. The calls are run in parallel.
func (g *Gateway) threadedBroadcast(name string, fn modules.RPCFunc) {
	g.log.Printf("INFO: broadcasting RPC \"%v\" to %v peers\n", handlerName(name), len(g.peers))
	var wg sync.WaitGroup
	wg.Add(len(g.peers))
	id := g.mu.RLock()
	for peer := range g.peers {
		// contact each peer in a separate thread
		go func(peer modules.NetAddress) {
			err := g.streamRPC(peer, name, fn)
			if err != nil {
				g.log.Printf("WARN: broadcast: calling RPC \"%v\" on peer %v returned error: %v\n", handlerName(name), peer, err)
			}
			wg.Done()
		}(peer)
	}
	g.mu.RUnlock(id)
	wg.Wait()
}
