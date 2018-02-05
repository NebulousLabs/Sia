package siatest

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

type (
	// GroupParams is a helper struct to make creating TestGroups easier.
	GroupParams struct {
		Hosts   int // number of hosts to create
		Renters int // number of renters to create
		Miners  int // number of miners to create
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

var (
	// defaultAllowance is the allowance used for the group's renters
	defaultAllowance = modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(1e3),
		Hosts:       5,
		Period:      50,
		RenewWindow: 10,
	}
)

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
	// Add storage to hosts
	if err := addStorageFolderToHosts(tg.hosts); err != nil {
		return nil, err
	}
	// Announce hosts
	if err := announceHosts(tg.hosts); err != nil {
		return nil, err
	}
	// Mine a block to get the announcements confirmed
	if err := miner.MineBlock(); err != nil {
		return nil, err
	}
	// Block until all hosts show up as active in the renters' hostdbs
	if err := hostsInRenterDBCheck(miner, tg.renters, len(tg.hosts)); err != nil {
		return nil, err
	}
	// Set renter allowances
	if err := setRenterAllowances(tg.renters); err != nil {
		return nil, err
	}
	// Wait for all the renters to form contracts
	if err := waitForContracts(miner, tg.renters, tg.hosts); err != nil {
		return nil, err
	}
	// Make sure all nodes are synced
	if err := synchronizationCheck(miner, tg.nodes); err != nil {
		return nil, err
	}
	return tg, nil
}

// NewGroupFromTemplate will create hosts, renters and miners according to the
// settings in groupParams.
func NewGroupFromTemplate(groupParams GroupParams) (*TestGroup, error) {
	var params []node.NodeParams
	// Create host params
	for i := 0; i < groupParams.Hosts; i++ {
		params = append(params, node.Host(randomDir()))
	}
	// Create renter params
	for i := 0; i < groupParams.Renters; i++ {
		params = append(params, node.Renter(randomDir()))
	}
	// Create miner params
	for i := 0; i < groupParams.Miners; i++ {
		params = append(params, Miner(randomDir()))
	}
	return NewGroup(params...)
}

// addStorageFolderToHosts adds a single storage folder to each host.
func addStorageFolderToHosts(hosts map[*TestNode]struct{}) error {
	errors := make([]error, len(hosts))
	wg := new(sync.WaitGroup)
	i := 0
	// The following api call is very slow. Using multiple threads speeds that
	// process up a lot.
	for host := range hosts {
		wg.Add(1)
		go func(i int, host *TestNode) {
			errors[i] = host.HostStorageFoldersAddPost(host.Dir, 1048576)
			wg.Done()
		}(i, host)
		i++
	}
	wg.Wait()
	return build.ComposeErrors(errors...)
}

// announceHosts adds storage to each host and announces them to the group
func announceHosts(hosts map[*TestNode]struct{}) error {
	for host := range hosts {
		if err := host.HostAcceptingContractsPost(true); err != nil {
			return err
		}
		if err := host.HostAnnouncePost(); err != nil {
			return err
		}
	}
	return nil
}

// fullyConnectNodes takes a list of nodes and connects all their gateways
func fullyConnectNodes(nodes []*TestNode) error {
	// Fully connect the nodes
	for i, nodeA := range nodes {
		for _, nodeB := range nodes[i+1:] {
			if err := nodeA.GatewayConnectPost(nodeB.GatewayAddress()); err != nil && err != client.ErrPeerExists {
				return build.ExtendErr("failed to connect to peer", err)
			}
		}
	}
	return nil
}

// fundNodes uses the funds of a miner node to fund all the nodes of the group
func fundNodes(miner *TestNode, nodes map[*TestNode]struct{}) error {
	// Get the miner's balance
	wg, err := miner.WalletGet()
	if err != nil {
		return err
	}
	// Send txnsPerNode outputs to each node
	txnsPerNode := uint64(25)
	scos := make([]types.SiacoinOutput, 0, uint64(len(nodes))*txnsPerNode)
	funding := wg.ConfirmedSiacoinBalance.Div64(uint64(len(nodes))).Div64(txnsPerNode + 1)
	for node := range nodes {
		wag, err := node.WalletAddressGet()
		if err != nil {
			return err
		}
		for i := uint64(0); i < txnsPerNode; i++ {
			scos = append(scos, types.SiacoinOutput{
				Value:      funding,
				UnlockHash: wag.Address,
			})
		}
	}
	// Send the transaction
	_, err = miner.WalletSiacoinsMultiPost(scos)
	if err != nil {
		return err
	}
	// Mine the transactions
	if err := miner.MineBlock(); err != nil {
		return err
	}
	return nil
}

// hostsInRenterDBCheck makes sure that all the renters see numHosts hosts in
// their database.
func hostsInRenterDBCheck(miner *TestNode, renters map[*TestNode]struct{}, numHosts int) error {
	for renter := range renters {
		err := Retry(100, 100*time.Millisecond, func() error {
			hdag, err := renter.HostDbActiveGet()
			if err != nil {
				return err
			}
			if len(hdag.Hosts) != numHosts {
				if err := miner.MineBlock(); err != nil {
					return err
				}
				return errors.New("renter doesn't have enough active hosts in hostdb")
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

// randomDir is a helper functions that returns a random directory path
func randomDir() string {
	dir, err := TestDir(strconv.Itoa(fastrand.Intn(math.MaxInt32)))
	if err != nil {
		panic(build.ExtendErr("failed to create testing directory", err))
	}
	return dir
}

// setRenterAllowances sets the allowance of each renter
func setRenterAllowances(renters map[*TestNode]struct{}) error {
	for renter := range renters {
		if err := renter.RenterPost(defaultAllowance); err != nil {
			return err
		}
	}
	return nil
}

// synchronizationCheck makes sure that all the nodes are synced and follow the
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

// waitForContracts waits until the renters have formed contracts with the
// hosts in the group.
func waitForContracts(miner *TestNode, renters map[*TestNode]struct{}, hosts map[*TestNode]struct{}) error {
	expectedContracts := defaultAllowance.Hosts
	if uint64(len(hosts)) < expectedContracts {
		expectedContracts = uint64(len(hosts))
	}
	for renter := range renters {
		numRetries := 0
		err := build.Retry(1000, 100*time.Millisecond, func() error {
			if numRetries%10 == 0 {
				if err := miner.MineBlock(); err != nil {
					return err
				}
			}
			rc, err := renter.RenterContractsGet()
			if err != nil {
				return err
			}
			if uint64(len(rc.Contracts)) < expectedContracts {
				return errors.New("Renter hasn't formed enough contracts")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the group and all its nodes
func (tg *TestGroup) Close() (err error) {
	for n := range tg.nodes {
		err = build.ComposeErrors(err, n.Close())
	}
	return
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
