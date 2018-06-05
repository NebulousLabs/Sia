package siatest

import (
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/node/api/client"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/errors"
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
	// DefaultAllowance is the allowance used for the group's renters
	DefaultAllowance = modules.Allowance{
		Funds:       types.SiacoinPrecision.Mul64(1e3),
		Hosts:       5,
		Period:      50,
		RenewWindow: 24,
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
			return nil, errors.AddContext(err, "failed to create clean node")
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

	// Get a miner and mine some blocks to generate coins
	if len(tg.miners) == 0 {
		return nil, errors.New("cannot fund group without miners")
	}
	miner := tg.Miners()[0]
	for i := types.BlockHeight(0); i <= types.MaturityDelay+types.TaxHardforkHeight; i++ {
		if err := miner.MineBlock(); err != nil {
			return nil, errors.AddContext(err, "failed to mine block for funding")
		}
	}
	// Fully connect nodes
	return tg, tg.setupNodes(tg.hosts, tg.nodes, tg.renters)
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
	errs := make([]error, len(hosts))
	wg := new(sync.WaitGroup)
	i := 0
	// The following api call is very slow. Using multiple threads speeds that
	// process up a lot.
	for host := range hosts {
		wg.Add(1)
		go func(i int, host *TestNode) {
			errs[i] = host.HostStorageFoldersAddPost(host.Dir, 1048576)
			wg.Done()
		}(i, host)
		i++
	}
	wg.Wait()
	return errors.Compose(errs...)
}

// announceHosts adds storage to each host and announces them to the group
func announceHosts(hosts map[*TestNode]struct{}) error {
	for host := range hosts {
		if err := host.HostModifySettingPost(client.HostParamAcceptingContracts, true); err != nil {
			return errors.AddContext(err, "failed to set host to accepting contracts")
		}
		if err := host.HostAnnouncePost(); err != nil {
			return errors.AddContext(err, "failed to announce host")
		}
	}
	return nil
}

// fullyConnectNodes takes a list of nodes and connects all their gateways
func fullyConnectNodes(nodes []*TestNode) error {
	// Fully connect the nodes
	for i, nodeA := range nodes {
		for _, nodeB := range nodes[i+1:] {
			err := build.Retry(100, 100*time.Millisecond, func() error {
				if err := nodeA.GatewayConnectPost(nodeB.GatewayAddress()); err != nil && err != client.ErrPeerExists {
					return errors.AddContext(err, "failed to connect to peer")
				}
				isPeer1, err1 := nodeA.hasPeer(nodeB)
				isPeer2, err2 := nodeB.hasPeer(nodeA)
				if err1 != nil || err2 != nil {
					return build.ExtendErr("couldn't determine if nodeA and nodeB are connected",
						errors.Compose(err1, err2))
				}
				if isPeer1 && isPeer2 {
					return nil
				}
				return errors.New("nodeA and nodeB are not peers of each other")
			})
			if err != nil {
				return err
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
		return errors.AddContext(err, "failed to get miner's balance")
	}
	// Send txnsPerNode outputs to each node
	txnsPerNode := uint64(25)
	scos := make([]types.SiacoinOutput, 0, uint64(len(nodes))*txnsPerNode)
	funding := wg.ConfirmedSiacoinBalance.Div64(uint64(len(nodes))).Div64(txnsPerNode + 1)
	for node := range nodes {
		wag, err := node.WalletAddressGet()
		if err != nil {
			return errors.AddContext(err, "failed to get wallet address")
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
		return errors.AddContext(err, "failed to send funding txn")
	}
	// Mine the transactions
	if err := miner.MineBlock(); err != nil {
		return errors.AddContext(err, "failed to mine funding txn")
	}
	// Make sure every node has at least one confirmed transaction
	for node := range nodes {
		err := Retry(100, 100*time.Millisecond, func() error {
			wtg, err := node.WalletTransactionsGet(0, math.MaxInt32)
			if err != nil {
				return err
			}
			if len(wtg.ConfirmedTransactions) == 0 {
				return errors.New("confirmed transactions should be greater than 0")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// hostsInRenterDBCheck makes sure that all the renters see all hosts in their
// database.
func hostsInRenterDBCheck(miner *TestNode, renters map[*TestNode]struct{}, hosts map[*TestNode]struct{}) error {
	for renter := range renters {
		if renter.params.SkipHostDiscovery {
			continue
		}
		for host := range hosts {
			numRetries := 0
			err := Retry(100, 100*time.Millisecond, func() error {
				numRetries++
				if renter == host {
					// We don't care if the renter is also a host.
					return nil
				}
				// Check if the renter has the host in its db.
				err := errors.AddContext(renter.KnowsHost(host), "renter doesn't know host")
				if err != nil && numRetries%10 == 0 {
					return errors.Compose(err, miner.MineBlock())
				}
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return build.ExtendErr("not all renters can see all hosts", err)
			}
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
		panic(errors.AddContext(err, "failed to create testing directory"))
	}
	return dir
}

// setRenterAllowances sets the allowance of each renter
func setRenterAllowances(renters map[*TestNode]struct{}) error {
	for renter := range renters {
		// Set allowance
		if renter.params.SkipSetAllowance {
			continue
		}
		allowance := DefaultAllowance
		if !reflect.DeepEqual(renter.params.Allowance, modules.Allowance{}) {
			allowance = renter.params.Allowance
		}
		if err := renter.RenterPostAllowance(allowance); err != nil {
			return err
		}
	}
	return nil
}

// synchronizationCheck makes sure that all the nodes are synced and follow the
func synchronizationCheck(nodes map[*TestNode]struct{}) error {
	// Get node with longest chain.
	var longestChainNode *TestNode
	var longestChain types.BlockHeight
	for n := range nodes {
		ncg, err := n.ConsensusGet()
		if err != nil {
			return err
		}
		if ncg.Height > longestChain {
			longestChain = ncg.Height
			longestChainNode = n
		}
	}
	lcg, err := longestChainNode.ConsensusGet()
	if err != nil {
		return err
	}
	// Loop until all the blocks have the same CurrentBlock.
	for n := range nodes {
		err := Retry(600, 100*time.Millisecond, func() error {
			ncg, err := n.ConsensusGet()
			if err != nil {
				return err
			}
			// If the CurrentBlock's match we are done.
			if lcg.CurrentBlock == ncg.CurrentBlock {
				return nil
			}
			// If the miner's height is greater than the node's we need to
			// wait a bit longer for them to sync.
			if lcg.Height != ncg.Height {
				return errors.New("blockHeight doesn't match")
			}
			// If the miner's height is smaller than the node's we need a
			// bit longer for them to sync.
			if lcg.CurrentBlock != ncg.CurrentBlock {
				return errors.New("ids don't match")
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
	// Create a map for easier public key lookups.
	hostMap := make(map[string]struct{})
	for host := range hosts {
		pk, err := host.HostPublicKey()
		if err != nil {
			return build.ExtendErr("failed to build hostMap", err)
		}
		hostMap[string(pk.Key)] = struct{}{}
	}
	// each renter is supposed to have at least expectedContracts with hosts
	// from the hosts map.
	for renter := range renters {
		numRetries := 0
		// Get expected number of contracts for this renter.
		rg, err := renter.RenterGet()
		if err != nil {
			return err
		}
		// If there are less hosts in the group than we need we need to adjust
		// our expectations.
		expectedContracts := rg.Settings.Allowance.Hosts
		if uint64(len(hosts)) < expectedContracts {
			expectedContracts = uint64(len(hosts))
		}
		// Check if number of contracts is sufficient.
		err = Retry(1000, 100, func() error {
			numRetries++
			contracts := uint64(0)
			// Get the renter's contracts.
			rc, err := renter.RenterContractsGet()
			if err != nil {
				return err
			}
			// Count number of contracts
			for _, c := range rc.Contracts {
				if _, exists := hostMap[string(c.HostPublicKey.Key)]; exists {
					contracts++
				}
			}
			// Check if number is sufficient
			if contracts < expectedContracts {
				if numRetries%10 == 0 {
					if err := miner.MineBlock(); err != nil {
						return err
					}
				}
				return errors.New("renter hasn't formed enough contracts")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	// Mine of 1 final block to ensure contracts are mined and show
	// up in a block
	return miner.MineBlock()
}

// AddNodeN adds n nodes of a given template to the group.
func (tg *TestGroup) AddNodeN(np node.NodeParams, n int) error {
	nps := make([]node.NodeParams, n)
	for i := 0; i < n; i++ {
		nps[i] = np
	}
	return tg.AddNodes(nps...)
}

// AddNodes creates a node and adds it to the group.
func (tg *TestGroup) AddNodes(nps ...node.NodeParams) error {
	newNodes := make(map[*TestNode]struct{})
	newHosts := make(map[*TestNode]struct{})
	newRenters := make(map[*TestNode]struct{})
	for _, np := range nps {
		// Create the nodes and add them to the group.
		if np.Dir == "" {
			np.Dir = randomDir()
		}
		node, err := NewCleanNode(np)
		if err != nil {
			return build.ExtendErr("failed to create host", err)
		}
		// Add node to nodes
		tg.nodes[node] = struct{}{}
		// Add node to hosts
		if np.Host != nil || np.CreateHost {
			tg.hosts[node] = struct{}{}
			newHosts[node] = struct{}{}
		}
		// Add node to renters
		if np.Renter != nil || np.CreateRenter {
			tg.renters[node] = struct{}{}
			newRenters[node] = struct{}{}
		}
		// Add node to miners
		if np.Miner != nil || np.CreateMiner {
			tg.miners[node] = struct{}{}
		}
		newNodes[node] = struct{}{}
	}

	return tg.setupNodes(newHosts, newNodes, newRenters)
}

// setupNodes does the set up required for creating a test group
// and add nodes to a group
func (tg *TestGroup) setupNodes(setHosts, setNodes, setRenters map[*TestNode]struct{}) error {
	// Find richest miner.
	var miner *TestNode
	var balance types.Currency
	for m := range tg.miners {
		wg, err := m.WalletGet()
		if err != nil {
			return errors.New("failed to find richest miner")
		}
		if wg.ConfirmedSiacoinBalance.Cmp(balance) > 0 {
			miner = m
			balance = wg.ConfirmedSiacoinBalance
		}
	}
	// Get all the nodes.
	nodes := mapToSlice(tg.nodes)
	if err := fullyConnectNodes(nodes); err != nil {
		return build.ExtendErr("failed to fully connect nodes", err)
	}
	// Make sure the new nodes are synced.
	if err := synchronizationCheck(tg.nodes); err != nil {
		return build.ExtendErr("synchronization check 1 failed", err)
	}
	// Fund nodes.
	if err := fundNodes(miner, setNodes); err != nil {
		return build.ExtendErr("failed to fund new hosts", err)
	}
	// Add storage to host
	if err := addStorageFolderToHosts(setHosts); err != nil {
		return build.ExtendErr("failed to add storage to hosts", err)
	}
	// Announce host
	if err := announceHosts(setHosts); err != nil {
		return build.ExtendErr("failed to announce hosts", err)
	}
	// Mine a block to get the announcements confirmed
	if err := miner.MineBlock(); err != nil {
		return build.ExtendErr("failed to mine host announcements", err)
	}
	// Block until the hosts show up as active in the renters' hostdbs
	if err := hostsInRenterDBCheck(miner, tg.renters, tg.hosts); err != nil {
		return build.ExtendErr("renter database check failed", err)
	}
	// Set renter allowances
	if err := setRenterAllowances(setRenters); err != nil {
		return build.ExtendErr("failed to set renter allowance", err)
	}
	// Wait for all the renters to form contracts if the haven't got enough
	// contracts already.
	if err := waitForContracts(miner, tg.renters, tg.hosts); err != nil {
		return build.ExtendErr("renters failed to form contracts", err)
	}
	// Make sure all nodes are synced
	if err := synchronizationCheck(tg.nodes); err != nil {
		return build.ExtendErr("synchronization check 2 failed", err)
	}
	return nil
}

// SetRenterAllowance finished the setup for the renter test node
func (tg *TestGroup) SetRenterAllowance(renter *TestNode, allowance modules.Allowance) error {
	if _, ok := tg.renters[renter]; !ok {
		return errors.New("Can not set allowance for renter not in test group")
	}
	miner := mapToSlice(tg.miners)[0]
	r := make(map[*TestNode]struct{})
	r[renter] = struct{}{}
	// Set renter allowances
	renter.params.SkipSetAllowance = false
	if err := setRenterAllowances(r); err != nil {
		return build.ExtendErr("failed to set renter allowance", err)
	}
	// Wait for all the renters to form contracts if the haven't got enough
	// contracts already.
	if err := waitForContracts(miner, r, tg.hosts); err != nil {
		return build.ExtendErr("renters failed to form contracts", err)
	}
	// Make sure all nodes are synced
	if err := synchronizationCheck(tg.nodes); err != nil {
		return build.ExtendErr("synchronization check 2 failed", err)
	}
	return nil
}

// Close closes the group and all its nodes. Closing a node is usually a slow
// process, but we can speed it up a lot by closing each node in a separate
// goroutine.
func (tg *TestGroup) Close() error {
	wg := new(sync.WaitGroup)
	errs := make([]error, len(tg.nodes))
	i := 0
	for n := range tg.nodes {
		wg.Add(1)
		go func(i int, n *TestNode) {
			errs[i] = n.Close()
			wg.Done()
		}(i, n)
		i++
	}
	wg.Wait()
	return errors.Compose(errs...)
}

// RemoveNode removes a node from the group and shuts it down.
func (tg *TestGroup) RemoveNode(tn *TestNode) error {
	// Remote node from all data structures.
	delete(tg.nodes, tn)
	delete(tg.hosts, tn)
	delete(tg.renters, tn)
	delete(tg.miners, tn)

	// Close node.
	return tn.StopNode()
}

// StartNode starts a node from the group that has previously been stopped.
func (tg *TestGroup) StartNode(tn *TestNode) error {
	if _, exists := tg.nodes[tn]; !exists {
		return errors.New("cannot start node that's not part of the group")
	}
	err := tn.StartNode()
	if err != nil {
		return err
	}
	if err := fullyConnectNodes(tg.Nodes()); err != nil {
		return err
	}
	return synchronizationCheck(tg.nodes)
}

// StopNode stops a node of a group.
func (tg *TestGroup) StopNode(tn *TestNode) error {
	if _, exists := tg.nodes[tn]; !exists {
		return errors.New("cannot stop node that's not part of the group")
	}
	return tn.StopNode()
}

// Sync syncs the node of the test group
func (tg *TestGroup) Sync() error {
	return synchronizationCheck(tg.nodes)
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
