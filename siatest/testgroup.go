package siatest

import (
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

type (
	// GroupParams is a helper struct to make creating TestGroups easier.
	GroupParams struct {
		hosts   int // number of hosts to create
		renters int // number of renters to create
		miners  int // number of miners to create
	}

	// TestGroup is a group of of TestNodes that are funded, synced and ready
	// for upload, download and mining depending on their configuration
	TestGroup struct {
		nodes   map[*TestNode]struct{}
		hosts   map[*TestNode]struct{}
		renters map[*TestNode]struct{}
		miners  map[*TestNode]struct{}

		dir string
	}
)

// Close closes the group and all its nodes
func (tg *TestGroup) Close() (err error) {
	for n := range tg.nodes {
		err = build.ComposeErrors(err, n.Close())
	}
	return
}

// fullyConnectNodes takes a list of nodes and connects all their gateways
func fullyConnectNodes(nodes []*TestNode) error {
	// Fully connect the nodes
	for i, nodeA := range nodes {
		for _, nodeB := range nodes[i+1:] {
			if err := nodeA.GatewayConnectPost(nodeB.GatewayAddress()); err != nil {
				return build.ExtendErr("failed to connect to peer", err)
			}
		}
	}
	return nil
}

// fundNodes uses the funds of a miner node to fund all the nodes of the group
func fundNodes(miner *TestNode, nodes map[*TestNode]struct{}) error {
	// Fund all the nodes equally
	wg, err := miner.WalletGet()
	if err != nil {
		return err
	}
	// Calculate the funding for each node.
	// Add +1 to avoid rounding errors that might lead to insufficient funds.
	funding := wg.ConfirmedSiacoinBalance.Div64(uint64(len(nodes)) + 1)
	// Prepare the transaction outputs
	scos := make([]types.SiacoinOutput, 0, len(nodes))
	for node := range nodes {
		wag, err := node.WalletAddressGet()
		if err != nil {
			return err
		}
		scos = append(scos, types.SiacoinOutput{
			Value:      funding,
			UnlockHash: wag.Address,
		})
	}
	// Send the coins to the outputs
	_, err = miner.WalletSiacoinsMultiPost(scos)
	if err != nil {
		return err
	}
	// Mine the transaction
	if err := miner.MineBlock(); err != nil {
		return err
	}
	return nil
}

// NewGroup creates a group of TestNodes from node params. All the nodes will
// be connected, synced and funded. Hosts nodes are also announced.
func NewGroup(nodeParams ...node.NodeParams) (*TestGroup, error) {
	// Create and init group
	tg := &TestGroup{
		nodes:   make(map[*TestNode]struct{}),
		hosts:   make(map[*TestNode]struct{}),
		renters: make(map[*TestNode]struct{}),
		miners:  make(map[*TestNode]struct{}),
	}

	// Create node and add it to the correct groups
	nodes := make([]*TestNode, 0, len(nodeParams))
	for _, np := range nodeParams {
		node, err := NewCleanNode(np)
		if err != nil {
			return nil, err
		}
		// Add node to nodes
		tg.nodes[node] = struct{}{}
		nodes = append(nodes, node)
		// Add node to hosts
		if np.Host != nil || np.CreateHost {
			tg.hosts[node] = struct{}{}
		}
		// Add node to renters
		if np.Renter != nil || np.CreateRenter {
			tg.renters[node] = struct{}{}
		}
		// Add node to miners
		if np.Miner != nil || np.CreateMiner {
			tg.miners[node] = struct{}{}
		}
	}

	// Fully connect nodes
	if err := fullyConnectNodes(nodes); err != nil {
		return nil, err
	}

	// Get a miner and mine some blocks to generate coins
	if len(tg.miners) == 0 {
		return nil, errors.New("cannot fund group without miners")
	}
	miner := tg.Miners()[0]
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		if err := miner.MineBlock(); err != nil {
			return nil, err
		}
	}

	// Fund nodes
	if err := fundNodes(miner, tg.nodes); err != nil {
		return nil, err
	}

	// Set renter allowances
	if err := setRentersAllowance(tg.renters); err != nil {
		return nil, err
	}

	// TODO add host storage folder
	// TODO set hosts to accepting contracts
	// TODO announce host

	// TODO Mine blocks until all the hosts show up in the hostdbs of the renters.
	// TODO Make sure all renters formed contracts with the hosts

	// Make sure all nodes are synced
	if err := synchronizationCheck(miner, tg.nodes); err != nil {
		return nil, err
	}
	return tg, nil
}

// setRentersAllowance sets the allowance of each renter
func setRentersAllowance(renters map[*TestNode]struct{}) error {
	// Create a sane default allowance for all renters
	allowance := modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(100000),
		Hosts:       50,
		Period:      10,
		RenewWindow: 5,
	}
	for renter := range renters {
		renter.RenterPost(allowance)
	}
	return nil
}

// NewGroupFromTemplate will create hosts, renters and miners according to the
// settings in groupParams.
func NewGroupFromTemplate(groupParams GroupParams) (*TestGroup, error) {
	var params []node.NodeParams
	// Create host params
	for i := 0; i < groupParams.hosts; i++ {
		params = append(params, node.Host(randomDir()))
	}
	// Create renter params
	for i := 0; i < groupParams.renters; i++ {
		params = append(params, node.Renter(randomDir()))
	}
	// Create miner params
	for i := 0; i < groupParams.miners; i++ {
		params = append(params, Miner(randomDir()))
	}
	return NewGroup(params...)
}

// randomDir is a helper functions that returns a random directory path
func randomDir() string {
	dir, err := TestDir(strconv.Itoa(fastrand.Intn(math.MaxInt32)))
	if err != nil {
		panic(build.ExtendErr("failed to create testing directory", err))
	}
	return dir
}

// synchronizationCheck makes sure that all the nodes are synced and follow the
// same chain.
func synchronizationCheck(miner *TestNode, nodes map[*TestNode]struct{}) error {
	mcg, err := miner.ConsensusGet()
	if err != nil {
		return err
	}
	for node := range nodes {
		err := Retry(100, 100*time.Millisecond, func() error {
			ncg, err := node.ConsensusGet()
			if err != nil {
				return err
			}
			if mcg.CurrentBlock != ncg.CurrentBlock {
				return errors.New("the node's current block doesn't equal the miner's")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// mapToSlice converts a map of TestNodes into a slice
func mapToSlice(m map[*TestNode]struct{}) []*TestNode {
	tns := make([]*TestNode, 0, len(m))
	for tn := range m {
		tns = append(tns, tn)
	}
	return tns
}

// Nodes returns all the nodes of the group
func (tg *TestGroup) Nodes() []*TestNode {
	return mapToSlice(tg.nodes)
}

// Hosts returns all the hosts of the group
func (tg *TestGroup) Hosts() []*TestNode {
	return mapToSlice(tg.hosts)
}

// Renters returns all the renters of the group
func (tg *TestGroup) Renters() []*TestNode {
	return mapToSlice(tg.renters)
}

// Miners returns all the miners of the group
func (tg *TestGroup) Miners() []*TestNode {
	return mapToSlice(tg.miners)
}
