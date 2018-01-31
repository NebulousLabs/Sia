package siatest

import (
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/build"
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

// randomDir is a helper functions that returns a random directory path
func randomDir() string {
	dir, err := TestDir(strconv.Itoa(fastrand.Intn(math.MaxInt32)))
	if err != nil {
		panic(build.ExtendErr("failed to create testing directory", err))
	}
	return dir
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
		params = append(params, node.Miner(randomDir()))
	}
	return NewGroup(params)
}

// NewGroup creates a group of TestNodes from node params. All the nodes will
// be connected, synced and funded. Hosts nodes are also announced.
func NewGroup(nodeParams []node.NodeParams) (*TestGroup, error) {
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

	// Fully connect the nodes
	for i, nodeA := range nodes {
		for _, nodeB := range nodes[i+1:] {
			if err := nodeA.GatewayConnectPost(nodeB.GatewayAddress()); err != nil {
				return nil, build.ExtendErr("failed to connect to peer", err)
			}
		}
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

	// TODO Fund all the other nodes

	// TODO announce hosts and wait for them to show up in each other's hostdb

	// All nodes should be at the same block as the miner
	mcg, err := miner.ConsensusGet()
	if err != nil {
		return nil, err
	}
	for node := range tg.nodes {
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
			return nil, err
		}
	}
	return tg, nil
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
