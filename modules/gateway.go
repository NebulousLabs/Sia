package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

type GatewayInfo struct {
	Peers []network.Address
}

// A Gateway facilitates the interactions between the local node and remote
// nodes (peers). It relays incoming blocks and transactions to local modules,
// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
// it is responsible for ensuring that the local consensus set is consistent
// with the "network" consensus set.
type Gateway interface {
	// AcceptBlock takes a block and submits it to the state.
	AcceptBlock(consensus.Block) error

	// AcceptTransaction takes a transaction and submits it to the transaction
	// pool.
	AcceptTransaction(consensus.Transaction) error

	// Bootstrap joins the Sia network and establishes an initial peer list.
	Bootstrap(network.Address) error

	// AddPeer adds a peer to the Gateway's peer list. The peer
	// may be rejected. AddPeer is also an RPC.
	AddPeer(network.Address) error

	// RemovePeer removes a peer from the Gateway's peer list.
	RemovePeer(network.Address) error

	// Synchronize synchronizes the local consensus set with the sets of known
	// peers.
	Synchronize() error

	// RelayTransaction announces a transaction to all of the Gateway's
	// known peers.
	RelayTransaction(consensus.Transaction) error

	// AddMe is the RPC version of AddPeer. It is assumed that the supplied
	// peer is the peer making the RPC.
	AddMe(network.Address) error

	// SendBlocks is an RPC that returns a set of sequential blocks following
	// the most recent known block ID in of the 32 IDs provided. The number of
	// blocks returned is unspecified.
	SendBlocks([32]consensus.BlockID) ([]consensus.Block, error)

	// SharePeers is an RPC that returns a set of the Gateway's peers. The
	// number of peers returned is unspecified.
	SharePeers() ([]network.Address, error)

	// Info reports metadata about the Gateway.
	Info() GatewayInfo
}
