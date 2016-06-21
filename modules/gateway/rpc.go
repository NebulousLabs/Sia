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
	if err := g.threads.Add(); err != nil {
		return err
	}
	defer g.threads.Done()

	g.mu.RLock()
	peer, ok := g.peers[addr]
	g.mu.RUnlock()
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.handlers[handlerName(name)]; ok {
		build.Critical("RPC already registered: " + name)
	}
	g.handlers[handlerName(name)] = fn
}

// UnregisterRPC unregisters an RPC and removes the corresponding RPCFunc from
// g.handlers. Future calls to the RPC by peers will fail.
func (g *Gateway) UnregisterRPC(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.handlers[handlerName(name)]; !ok {
		build.Critical("RPC not registered: " + name)
	}
	delete(g.handlers, handlerName(name))
}

// RegisterConnectCall registers a name and RPCFunc to be called on a peer
// upon connecting.
func (g *Gateway) RegisterConnectCall(name string, fn modules.RPCFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.initRPCs[name]; ok {
		build.Critical("ConnectCall already registered: " + name)
	}
	g.initRPCs[name] = fn
}

// UnregisterConnectCall unregisters an on-connect call and removes the
// corresponding RPCFunc from g.initRPCs. Future connections to peers will not
// trigger the RPC to be called on them.
func (g *Gateway) UnregisterConnectCall(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.initRPCs[name]; !ok {
		build.Critical("ConnectCall not registered: " + name)
	}
	delete(g.initRPCs, name)
}

// threadedListenPeer listens for new streams on a peer connection and serves them via
// threadedHandleConn.
func (g *Gateway) threadedListenPeer(p *peer) {
	// Disconnect from the peer on gateway.Close or when the peer disconnects,
	// whichever comes first.
	peerCloseChan := make(chan struct{})
	defer close(peerCloseChan)
	// We register the goroutine with the ThreadGroup as it acquires a lock, which
	// may take some time.
	if g.threads.Add() != nil {
		return
	}
	go func() {
		defer g.threads.Done()
		select {
		case <-g.threads.StopChan():
		case <-peerCloseChan:
		}
		// Can't call Disconnect because it could return sync.ErrStopped.
		g.mu.Lock()
		delete(g.peers, p.NetAddress)
		g.mu.Unlock()
		if err := p.sess.Close(); err != nil {
			g.log.Debugf("WARN: error disconnecting from peer %q: %v", p.NetAddress, err)
		}
	}()

	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	for {
		conn, err := p.accept()
		if err != nil {
			g.log.Println("WARN: lost connection to peer", p.NetAddress)
			return
		}

		// it is the handler's responsibility to close the connection
		go g.threadedHandleConn(conn)
	}
}

// threadedHandleConn reads header data from a connection, then routes it to the
// appropriate handler for further processing.
func (g *Gateway) threadedHandleConn(conn modules.PeerConn) {
	defer conn.Close()

	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	var id rpcID
	if err := encoding.ReadObject(conn, &id, 8); err != nil {
		return
	}
	// call registered handler for this ID
	g.mu.RLock()
	fn, ok := g.handlers[id]
	g.mu.RUnlock()
	if !ok {
		g.log.Debugf("WARN: incoming conn %v requested unknown RPC \"%v\"", conn.RPCAddr(), id)
		return
	}
	g.log.Debugf("INFO: incoming conn %v requested RPC \"%v\"", conn.RPCAddr(), id)

	// call fn
	err := fn(conn)
	// don't log benign errors
	if err == modules.ErrDuplicateTransactionSet || err == modules.ErrBlockKnown {
		err = nil
	}
	if err != nil {
		g.log.Debugf("WARN: incoming RPC \"%v\" from conn %v failed: %v", id, conn.RPCAddr(), err)
	}
}

// Broadcast calls an RPC on all of the specified peers. The calls are run in
// parallel. Broadcasts are restricted to "one-way" RPCs, which simply write an
// object and disconnect. This is why Broadcast takes an interface{} instead of
// an RPCFunc.
func (g *Gateway) Broadcast(name string, obj interface{}, peers []modules.Peer) {
	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	g.log.Printf("INFO: broadcasting RPC %q to %v peers", name, len(peers))

	// only encode obj once, instead of using WriteObject
	enc := encoding.Marshal(obj)
	fn := func(conn modules.PeerConn) error {
		return encoding.WritePrefix(conn, enc)
	}

	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(addr modules.NetAddress) {
			defer wg.Done()
			err := g.RPC(addr, name, fn)
			if err != nil {
				g.log.Debugf("WARN: broadcasting RPC %q to peer %q failed (attempting again in 10 seconds): %v", name, addr, err)
				// try one more time before giving up
				select {
				case <-time.After(10 * time.Second):
				case <-g.threads.StopChan():
					return
				}
				err := g.RPC(addr, name, fn)
				if err != nil {
					g.log.Debugf("WARN: broadcasting RPC %q to peer %q failed twice: %v", name, addr, err)
				}
			}
		}(p.NetAddress)
	}
	wg.Wait()
}
