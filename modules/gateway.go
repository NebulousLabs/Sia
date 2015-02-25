package modules

import (
	"net"

	"github.com/NebulousLabs/Sia/consensus"
)

// A NetAddress contains the information needed to contact a peer.
type NetAddress string

// Host returns the NetAddress' IP.
func (na NetAddress) Host() string {
	host, _, _ := net.SplitHostPort(string(na))
	return host
}

// Port returns the NetAddress' port number.
func (na NetAddress) Port() string {
	_, port, _ := net.SplitHostPort(string(na))
	return port
}

type GatewayInfo struct {
	Peers []NetAddress
}

// A Gateway facilitates the interactions between the local node and remote
// nodes (peers). It relays incoming blocks and transactions to local modules,
// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
// it is responsible for ensuring that the local consensus set is consistent
// with the "network" consensus set.
type Gateway interface {
	// Bootstrap joins the Sia network and establishes an initial peer list.
	Bootstrap(NetAddress) error

	// AddPeer adds a peer to the Gateway's peer list. The peer
	// may be rejected. AddPeer is also an RPC.
	AddPeer(NetAddress) error

	// RemovePeer removes a peer from the Gateway's peer list.
	RemovePeer(NetAddress) error

	// Synchronize synchronizes the local consensus set with the sets of known
	// peers.
	Synchronize() error

	// RelayBlock accepts a block and submits it to the state, broadcasting it
	// to the network if it's valid and on the current longest fork.
	RelayBlock(consensus.Block) error

	// RelayTransaction announces a transaction to all of the Gateway's
	// known peers.
	RelayTransaction(consensus.Transaction) error

	// AddMe is the RPC version of AddPeer. It is assumed that the supplied
	// peer is the peer making the RPC.
	AddMe(NetAddress) error

	// SendBlocks is an RPC that returns a set of sequential blocks following
	// the most recent known block ID in of the 32 IDs provided. The number of
	// blocks returned is unspecified.
	SendBlocks([32]consensus.BlockID) ([]consensus.Block, error)

	// SharePeers is an RPC that returns a set of the Gateway's peers. The
	// number of peers returned is unspecified.
	SharePeers() ([]NetAddress, error)

	// Info reports metadata about the Gateway.
	Info() GatewayInfo
}
