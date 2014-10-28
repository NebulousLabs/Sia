package sia

import (
	"errors"
	"net"
	"net/rpc/jsonrpc"
	"strconv"
	"time"
)

const timeout = time.Second * 5

// A NetAddress contains the information needed to contact a peer over the
// Internet.
type NetAddress struct {
	Host string
	Port uint16
}

// TBD
var BootstrapPeers = []NetAddress{}

// An RPCServer handles RPCs. This implementation currently uses JSON over TCP.
type RPCServer struct {
	listener    net.Listener
	addressbook map[NetAddress]struct{}
}

func NewRPCServer(port uint16) (rpcs *RPCServer, err error) {
	tcpServ, err := net.Listen("tcp", ":"+strconv.Itoa(int(port)))
	if err != nil {
		return
	}
	rpcs = &RPCServer{
		listener:    tcpServ,
		addressbook: make(map[NetAddress]struct{}),
	}
	go rpcs.handleRPC()
	return
}

// Close closes the TCP connection used by the server. This will cause the
// handleRPC goroutine to terminate.
func (rpcs *RPCServer) Close() {
	rpcs.listener.Close()
}

// handleRPC runs in the background, accepting incoming RPCs and serving them.
// handleRPC will return after RPCServer.Close() is called, because the
// Accept() call will fail.
func (rpcs *RPCServer) handleRPC() {
	for {
		conn, err := rpcs.listener.Accept()
		if err != nil {
			return
		}
		go jsonrpc.ServeConn(conn)
	}
}

// RPC makes a remote procedure call on a NetAddress.
func (rpcs *RPCServer) RPC(addr NetAddress, proc string, args interface{}, resp interface{}) error {
	conn, err := jsonrpc.Dial("tcp", net.JoinHostPort(addr.Host, strconv.Itoa(int(addr.Port))))
	if err != nil {
		return err
	}
	defer conn.Close()

	select {
	case call := <-conn.Go(proc, args, resp, nil).Done:
		return call.Error
	case <-time.After(timeout):
		return errors.New("request timed out")
	}
}

// PeerList is an RPC that fills 'addr' with at most 'num' peers known to the RPCServer.
// TODO: add a random set of peers (map iteration may already handle this...)
func (rpcs *RPCServer) PeerList(num int, addr *map[NetAddress]struct{}) error {
	if num > 10 {
		num = 10
	}
	for i := range rpcs.addressbook {
		if num <= 0 {
			break
		}
		(*addr)[i] = struct{}{}
		num--
	}
	return nil
}

// Bootstrap calls Request on a predefined set of peers in order to build up an
// initial peer list. It returns the number of peers added.
func (rpcs *RPCServer) Bootstrap() int {
	n := len(rpcs.addressbook)
	for _, host := range BootstrapPeers {
		rpcs.RPC(host, "RPCServer.PeerList", 10, &rpcs.addressbook)
	}
	return len(rpcs.addressbook) - n
}
