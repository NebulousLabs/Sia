package consensus

import (
	"os"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/blockdb"
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// The State is the object responsible for tracking the current status of the
// blockchain. Broadly speaking, it is responsible for maintaining consensus.
// It accepts blocks and constructs a blockchain, forking when necessary.
type State struct {
	// The blockRoot is the block node that contains the genesis block.
	blockRoot *blockNode

	// blockMap and badBlocks keep track of seen blocks. blockMap holds all
	// valid blocks, including those not on the main blockchain. badBlocks
	// is a "blacklist" of blocks known to be invalid.
	blockMap  map[types.BlockID]*blockNode
	badBlocks map[types.BlockID]struct{}

	// The currentPath is the longest known blockchain.
	currentPath []types.BlockID

	// These are the consensus variables, referred to as the "consensus set."
	// All nodes with the same current path must have the same consensus set.
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

	// Updates to the state are stored as a list, pointing to the block nodes
	// that were added and removed at each step. Modules subscribed to the
	// state will receive the changes in order that they occur.
	revertUpdates [][]*blockNode
	applyUpdates  [][]*blockNode
	subscriptions []chan struct{}

	// block database, used for saving/loading the current path
	db blockdb.DB

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
func New(saveDir string) (*State, error) {
	// Create the State object.
	s := &State{
		blockMap:  make(map[types.BlockID]*blockNode),
		badBlocks: make(map[types.BlockID]struct{}),

		currentPath: make([]types.BlockID, 1),

		siacoinOutputs:        make(map[types.SiacoinOutputID]types.SiacoinOutput),
		fileContracts:         make(map[types.FileContractID]types.FileContract),
		siafundOutputs:        make(map[types.SiafundOutputID]types.SiafundOutput),
		delayedSiacoinOutputs: make(map[types.BlockHeight]map[types.SiacoinOutputID]types.SiacoinOutput),

		mu: sync.New(1*time.Second, 1),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := types.Block{
		Timestamp: types.GenesisTimestamp,
	}
	s.blockRoot = &blockNode{
		block:  genesisBlock,
		target: types.RootTarget,
		depth:  types.RootDepth,

		diffsGenerated: true,
	}
	s.blockMap[genesisBlock.ID()] = s.blockRoot

	// Fill out the consensus information for the genesis block.
	s.currentPath[0] = genesisBlock.ID()
	s.siacoinOutputs[genesisBlock.MinerPayoutID(0)] = types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.ZeroUnlockHash,
	}
	s.siafundOutputs[types.SiafundOutputID{0}] = types.SiafundOutput{
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
		s.db = blockdb.NilDB
		return s, nil
	}

	// Otherwise, try to load an existing database from disk.
	err = s.load(saveDir)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Close safely closes the block database.
func (s *State) Close() error {
	return s.db.Close()
}

// RLock will readlock the state.
func (s *State) RLock() int {
	return s.mu.RLock()
}

// RUnlock will readunlock the state.
func (s *State) RUnlock(id int) {
	s.mu.RUnlock(id)
}
