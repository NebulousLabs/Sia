package gateway

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// rpcID is an 8-byte signature that is added to all RPCs to tell the gatway
// what to do with the RPC.
type rpcID [8]byte

// String returns a string representation of an rpcID. Empty elements of rpcID
// will be encoded as spaces.
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

// managedRPC calls an RPC on the given address. managedRPC cannot be called on
// an address that the Gateway is not connected to.
func (g *Gateway) managedRPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	g.mu.RLock()
	peer, ok := g.peers[addr]
	g.mu.RUnlock()
	if !ok {
		return errors.New("can't call RPC on unconnected peer " + string(addr))
	}

	conn, err := peer.open()
	if err != nil {
		// peer probably disconnected without sending a shutdown signal;
		// disconnect from them
		g.log.Debugf("Could not initiate RPC with %v; disconnecting", addr)
		peer.sess.Close()
		g.mu.Lock()
		delete(g.peers, addr)
		g.mu.Unlock()
		return err
	}
	defer conn.Close()

	// write header
	conn.SetDeadline(time.Now().Add(rpcStdDeadline))
	if err := encoding.WriteObject(conn, handlerName(name)); err != nil {
		return err
	}
	conn.SetDeadline(time.Time{})
	// call fn
	return fn(conn)
}

// RPC calls an RPC on the given address. RPC cannot be called on an address
// that the Gateway is not connected to.
func (g *Gateway) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	if err := g.threads.Add(); err != nil {
		return err
	}
	defer g.threads.Done()
	return g.managedRPC(addr, name, fn)
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
	// threadedListenPeer registers to the peerTG instead of the primary thread
	// group because peer connections can be lifetime in length, but can also
	// be short-lived. The fact that they can be lifetime means that they can't
	// call threads.Add as they will block calls to threads.Flush. The fact
	// that they can be short-lived means that threads.OnStop is not a good
	// tool for closing out the threads. Instead, they register to peerTG,
	// which is cleanly closed upon gateway shutdown but will not block any
	// calls to threads.Flush()
	if g.peerTG.Add() != nil {
		return
	}
	defer g.peerTG.Done()

	// Spin up a goroutine to listen for a shutdown signal from both the peer
	// and from the gateway. In the event of either, close the session.
	connClosedChan := make(chan struct{})
	peerCloseChan := make(chan struct{})
	go func() {
		// Signal that the session has been successfully closed, and that this
		// goroutine has terminated.
		defer close(connClosedChan)

		// Listen for a stop signal.
		select {
		case <-g.threads.StopChan():
		case <-peerCloseChan:
		}

		// Close the session and remove p from the peer list.
		p.sess.Close()
		g.mu.Lock()
		delete(g.peers, p.NetAddress)
		g.mu.Unlock()
	}()

	for {
		conn, err := p.accept()
		if err != nil {
			g.log.Debugf("Peer connection with %v closed: %v\n", p.NetAddress, err)
			break
		}
		// Set the default deadline on the conn.
		err = conn.SetDeadline(time.Now().Add(rpcStdDeadline))
		if err != nil {
			g.log.Printf("Peer connection (%v) deadline could not be set: %v\n", p.NetAddress, err)
			continue
		}

		// The handler is responsible for closing the connection, though a
		// default deadline has been set.
		go g.threadedHandleConn(conn)
		if !g.managedSleep(peerRPCDelay) {
			break
		}
	}
	// Signal that the goroutine can shutdown.
	close(peerCloseChan)
	// Wait for confirmation that the goroutine has shut down before returning
	// and releasing the threadgroup registration.
	<-connClosedChan
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
	err := conn.SetDeadline(time.Now().Add(rpcStdDeadline))
	if err != nil {
		return
	}
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
	err = fn(conn)
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

	g.log.Debugf("INFO: broadcasting RPC %q to %v peers", name, len(peers))

	// only encode obj once, instead of using WriteObject
	enc := encoding.Marshal(obj)
	fn := func(conn modules.PeerConn) error {
		return encoding.WritePrefixedBytes(conn, enc)
	}

	var wg sync.WaitGroup
	for _, p := range peers {
		wg.Add(1)
		go func(addr modules.NetAddress) {
			defer wg.Done()
			err := g.managedRPC(addr, name, fn)
			if err != nil {
				g.log.Debugf("WARN: broadcasting RPC %q to peer %q failed (attempting again in 10 seconds): %v", name, addr, err)
				// try one more time before giving up
				select {
				case <-time.After(10 * time.Second):
				case <-g.threads.StopChan():
					return
				}
				err := g.managedRPC(addr, name, fn)
				if err != nil {
					g.log.Debugf("WARN: broadcasting RPC %q to peer %q failed twice: %v", name, addr, err)
				}
			}
		}(p.NetAddress)
	}
	wg.Wait()
}
