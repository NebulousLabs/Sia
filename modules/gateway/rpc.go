package gateway

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
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
	id := g.mu.RLock()
	peer, ok := g.peers[addr]
	g.mu.RUnlock(id)
	if !ok {
		return errors.New("can't call RPC on unconnected peer " + string(addr))
	}

	conn, err := peer.open()
	if err != nil {
		return err
	}
	defer conn.Close()

	// write header
	if err := encoding.WriteObject(conn, handlerName(name)); err != nil {
		return err
	}
	// call fn
	return fn(conn)
}

// RegisterRPC registers an RPCFunc as a handler for a given identifier. To
// call an RPC, use gateway.RPC, supplying the same identifier given to
// RegisterRPC. Identifiers should always use PascalCase. The first 8
// characters of an identifier should be unique, as the identifier used
// internally is truncated to 8 bytes.
func (g *Gateway) RegisterRPC(name string, fn modules.RPCFunc) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if _, ok := g.handlers[handlerName(name)]; ok {
		build.Critical("RPC already registered: " + name)
	}
	g.handlers[handlerName(name)] = fn
}

// UnregisterRPC unregisters an RPC and removes the corresponding RPCFunc from
// g.handlers. Future calls to the RPC by peers will fail.
func (g *Gateway) UnregisterRPC(name string) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if _, ok := g.handlers[handlerName(name)]; !ok {
		build.Critical("RPC not registered: " + name)
	}
	delete(g.handlers, handlerName(name))
}

// RegisterConnectCall registers a name and RPCFunc to be called on a peer
// upon connecting.
func (g *Gateway) RegisterConnectCall(name string, fn modules.RPCFunc) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if _, ok := g.initRPCs[name]; ok {
		build.Critical("ConnectCall already registered: " + name)
	}
	g.initRPCs[name] = fn
}

// UnregisterConnectCall unregisters an on-connect call and removes the
// corresponding RPCFunc from g.initRPCs. Future connections to peers will not
// trigger the RPC to be called on them.
func (g *Gateway) UnregisterConnectCall(name string) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if _, ok := g.initRPCs[name]; !ok {
		build.Critical("ConnectCall not registered: " + name)
	}
	delete(g.initRPCs, name)
}

// listenPeer listens for new streams on a peer connection and serves them via
// threadedHandleConn.
func (g *Gateway) listenPeer(p *peer) {
	for {
		conn, err := p.accept()
		if err != nil {
			g.log.Println("WARN: lost connection to peer", p.NetAddress)
			break
		}

		// it is the handler's responsibility to close the connection
		g.closeWG.Add(1)
		go func() {
			defer g.closeWG.Done()
			g.threadedHandleConn(conn)
		}()
	}
	g.Disconnect(p.NetAddress)
}

// threadedHandleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (g *Gateway) threadedHandleConn(conn modules.PeerConn) {
	defer conn.Close()
	var id rpcID
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		return
	}
	// call registered handler for this ID
	lockid := g.mu.RLock()
	fn, ok := g.handlers[id]
	g.mu.RUnlock(lockid)
	if !ok {
		g.log.Printf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RemoteAddr(), id)
		return
	}
	if build.DEBUG {
		g.log.Printf("INFO: incoming conn %v requested RPC \"%v\"", conn.RemoteAddr(), id)
	}

	// call fn
	err := fn(conn)
	// don't log benign errors
	if err == modules.ErrDuplicateTransactionSet || err == modules.ErrBlockKnown {
		err = nil
	}
	if err != nil {
		g.log.Printf("WARN: incoming RPC \"%v\" from conn %v failed: %v", id, conn.RemoteAddr(), err)
	}
}

// Broadcast calls an RPC on all of the specified peers. The calls are run in
// parallel. Broadcasts are restricted to "one-way" RPCs, which simply write an
// object and disconnect. This is why Broadcast takes an interface{} instead of
// an RPCFunc.
func (g *Gateway) Broadcast(name string, obj interface{}, peers []modules.Peer) {
	g.log.Printf("INFO: broadcasting RPC \"%v\" to %v peers", name, len(peers))

	// only encode obj once, instead of using WriteObject
	enc := encoding.Marshal(obj)
	fn := func(conn modules.PeerConn) error {
		return encoding.WritePrefix(conn, enc)
	}

	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, p := range peers {
		go func(addr modules.NetAddress) {
			err := g.RPC(addr, name, fn)
			if err != nil {
				// try one more time before giving up
				select {
				case <-time.After(10 * time.Second):
				case <-g.closeChan:
					return
				}
				g.RPC(addr, name, fn)
			}
			wg.Done()
		}(p.NetAddress)
	}
	wg.Wait()
}
