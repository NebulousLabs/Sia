package gateway

import (
	"github.com/NebulousLabs/Sia/modules"
)

type rpcID [8]byte

// handlerName truncates a string to 8 bytes. If len(name) < 8, the remaining
// bytes are 0. A handlerName is specified at the beginning of each network
// call, indicating which function should handle the connection.
func handlerName(name string) (id rpcID) {
	copy(id[:], name)
	return
}

// RPC establishes a TCP connection to the NetAddress, writes the RPC
// identifier, and then hands off the connection to fn. When fn returns, the
// connection is closed.
func (g *Gateway) RPC(addr modules.NetAddress, name string, fn modules.RPCFunc) error {
	conn, err := dial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	// write header
	if err := conn.WriteObject(handlerName(name)); err != nil {
		return err
	}
	return fn(conn)
}

// RegisterRPC registers a function as an RPC handler for a given identifier.
// To call an RPC, use gateway.RPC, supplying the same identifier given to
// RegisterRPC. Identifiers should always use PascalCase.
func (tcps *TCPServer) RegisterRPC(name string, fn modules.RPCFunc) {
	tcps.Lock()
	defer tcps.Unlock()
	tcps.handlerMap[handlerName(name)] = fn
}
