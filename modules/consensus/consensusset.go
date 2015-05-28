package consensus

import (
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// The State is the object responsible for tracking the current status of the
// blockchain. Broadly speaking, it is responsible for maintaining consensus.
// It accepts blocks and constructs a blockchain, forking when necessary.
type State struct {
	// fullVerification is a flag that tells the state whether or not to do
	// transaction verification while accepting a block. This should help speed
	// up loading blocks from memory.
	fullVerification bool

	// The blockRoot is the block node that contains the genesis block.
	blockRoot *blockNode

	// blockMap and dosBlocks keep track of seen blocks. blockMap holds all
	// valid blocks, including those not on the main blockchain. dosBlocks is a
	// "blacklist" of blocks known to be invalid, but expensive to prove
	// invalid.
	blockMap  map[types.BlockID]*blockNode
	dosBlocks map[types.BlockID]struct{}

	// The currentPath is the longest known blockchain.
	currentPath []types.BlockID

	// These are the consensus variables. All nodes with the same current path
	// will also have these variables matching.
	//
	// The siafundPool tracks the total number of siacoins that have been
	// taxed from file contracts. Unless a reorg occurs, the siafundPool
	// should never decrease.
	//
	// siacoinOutputs, fileContracts, and siafundOutputs keep track of the
	// unspent outputs and active contracts present in the current path. If an
	// output is spent or a contract expires, it is removed from the consensus
	// set. These objects may also be removed in the event of a reorg.
	//
	// delayedSiacoinOutputs are siacoin outputs that have been created in a
	// block, but are not allowed to be spent until a certain height. When
	// that height is reached, they are moved to the siacoinOutputs map.
	siafundPool           types.Currency
	siacoinOutputs        map[types.SiacoinOutputID]types.SiacoinOutput
	fileContracts         map[types.FileContractID]types.FileContract
	siafundOutputs        map[types.SiafundOutputID]types.SiafundOutput
	delayedSiacoinOutputs map[types.BlockHeight]map[types.SiacoinOutputID]types.SiacoinOutput

	// Modules subscribed to the consensus set will receive an ordered list of
	// changes that occur to the consensus set.
	consensusChanges []modules.ConsensusChange
	subscriptions    []chan struct{}

	// block database, used for saving/loading the current path
	db persist.DB

	// gateway, for receiving/relaying blocks to/from peers
	gateway modules.Gateway

	// Per convention, all exported functions in the consensus package can be
	// called concurrently. The state mutex helps to orchestrate thread safety.
	// To keep things simple, the entire state was chosen to have a single
	// mutex, as opposed to putting frequently accessed fields under separate
	// mutexes. The performance advantage was decided to be not worth the
	// complexity tradeoff.
	mu *sync.RWMutex
}

// New returns a new State, containing at least the genesis block. If there is
// an existing block database present in saveDir, it will be loaded. Otherwise,
// a new database will be created.
func New(gateway modules.Gateway, saveDir string) (*State, error) {
	if gateway == nil {
		return nil, errors.New("cannot have nil gateway")
	}

	// Create the State object.
	cs := &State{
		blockMap:  make(map[types.BlockID]*blockNode),
		dosBlocks: make(map[types.BlockID]struct{}),

		currentPath: make([]types.BlockID, 1),

		siacoinOutputs:        make(map[types.SiacoinOutputID]types.SiacoinOutput),
		fileContracts:         make(map[types.FileContractID]types.FileContract),
		siafundOutputs:        make(map[types.SiafundOutputID]types.SiafundOutput),
		delayedSiacoinOutputs: make(map[types.BlockHeight]map[types.SiacoinOutputID]types.SiacoinOutput),

		gateway: gateway,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := types.Block{
		Timestamp: types.GenesisTimestamp,
	}
	cs.blockRoot = &blockNode{
		block:  genesisBlock,
		target: types.RootTarget,
		depth:  types.RootDepth,

		diffsGenerated: true,
	}
	cs.blockMap[genesisBlock.ID()] = cs.blockRoot

	// Fill out the consensus information for the genesis block.
	cs.currentPath[0] = genesisBlock.ID()
	cs.siacoinOutputs[genesisBlock.MinerPayoutID(0)] = types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.ZeroUnlockHash,
	}
	cs.siafundOutputs[types.SiafundOutputID{0}] = types.SiafundOutput{
		Value:           types.NewCurrency64(types.SiafundCount),
		UnlockHash:      types.GenesisSiafundUnlockHash,
		ClaimUnlockHash: types.GenesisClaimUnlockHash,
	}

	// Create the consensus directory.
	err := os.MkdirAll(saveDir, 0700)
	if err != nil {
		return nil, err
	}

	// During short tests, use an in-memory database.
	if build.Release == "testing" && testing.Short() {
		cs.db = persist.NilDB
	} else {
		// Otherwise, try to load an existing database from disk.
		err = cs.load(saveDir)
		if err != nil {
			return nil, err
		}
	}

	// Register RPCs
	gateway.RegisterRPC("SendBlocks", cs.sendBlocks)
	gateway.RegisterRPC("RelayBlock", cs.RelayBlock)
	gateway.RegisterConnectCall("SendBlocks", cs.receiveBlocks)

	// spawn resynchronize loop
	go cs.threadedResynchronize()

	return cs, nil
}

// Close safely closes the block database.
func (cs *State) Close() error {
	return cs.db.Close()
}

// consensusSetHash returns the Merkle root of the current state of consensus.
func (cs *State) consensusSetHash() crypto.Hash {
	// Items of interest:
	// 1.	genesis block
	// 2.	current block id
	// 3.	current height
	// 4.	current target
	// 5.	current depth
	// 6.	earliest allowed timestamp of next block
	// 7.	current path, ordered by height.
	// 8.	unspent siacoin outputs, sorted by id.
	// 9.	open file contracts, sorted by id.
	// 10.	unspent siafund outputs, sorted by id.
	// 11.	delayed siacoin outputs, sorted by height, then sorted by id.

	// Create a slice of hashes representing all items of interest.
	tree := crypto.NewTree()
	tree.PushObject(cs.blockRoot.block)
	tree.PushObject(cs.height())
	tree.PushObject(cs.currentBlockNode().target)
	tree.PushObject(cs.currentBlockNode().depth)
	tree.PushObject(cs.currentBlockNode().earliestChildTimestamp())

	// Add all the blocks in the current path.
	for i := 0; i < len(cs.currentPath); i++ {
		tree.PushObject(cs.currentPath[types.BlockHeight(i)])
	}

	// Get the set of siacoin outputs in sorted order and add them.
	sortedUscos := cs.sortedUscoSet()
	for _, output := range sortedUscos {
		tree.PushObject(output)
	}

	// Sort the open contracts by ID.
	var openContracts crypto.HashSlice
	for contractID := range cs.fileContracts {
		openContracts = append(openContracts, crypto.Hash(contractID))
	}
	sort.Sort(openContracts)

	// Add the open contracts in sorted order.
	for _, id := range openContracts {
		tree.PushObject(id)
	}

	// Get the set of siafund outputs in sorted order and add them.
	for _, output := range cs.sortedUsfoSet() {
		tree.PushObject(output)
	}

	// Get the set of delayed siacoin outputs, sorted by maturity height then
	// sorted by id and add them.
	for i := types.BlockHeight(0); i <= cs.height(); i++ {
		var delayedOutputs crypto.HashSlice
		for id := range cs.delayedSiacoinOutputs[i] {
			delayedOutputs = append(delayedOutputs, crypto.Hash(id))
		}
		sort.Sort(delayedOutputs)

		for _, output := range delayedOutputs {
			tree.PushObject(output)
		}
	}

	return tree.Root()
}
