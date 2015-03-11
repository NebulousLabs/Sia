package consensus

import (
	"sync"
)

// The ZeroUnlockHash and ZeroCurrency are convenience variables.
var (
	ZeroUnlockHash = UnlockHash{0}
	ZeroCurrency   = NewCurrency64(0)
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
	blockMap  map[BlockID]*blockNode
	badBlocks map[BlockID]struct{}

	// The currentPath is the longest known blockchain.
	currentPath []BlockID

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
	siafundPool           Currency
	siacoinOutputs        map[SiacoinOutputID]SiacoinOutput
	fileContracts         map[FileContractID]FileContract
	siafundOutputs        map[SiafundOutputID]SiafundOutput
	delayedSiacoinOutputs map[BlockHeight]map[SiacoinOutputID]SiacoinOutput

	// subscriptions is a map containing a bunch of channels that are being
	// listend on by modules. An empty struct is thrown down the channel any
	// time that the consensus set of the state changes. subscriptionCounter
	// only ever increments, and prevents collisions in the map.
	subscriptions []chan struct{}

	// Per convention, all exported functions in the consensus package can be
	// called concurrently. The state mutex helps to orchestrate thread safety.
	// To keep things simple, the entire state was chosen to have a single
	// mutex, as opposed to putting frequently accessed fields under separate
	// mutexes. The performance advantage was decided to be not worth the
	// complexity tradeoff.
	mu sync.RWMutex
}

// createGenesisState returns a State containing only the genesis block. It
// takes arguments instead of using global constants to make testing easier.
func createGenesisState(genesisTime Timestamp, fundUnlockHash UnlockHash, claimUnlockHash UnlockHash) (s *State) {
	// Create a new state and initialize the maps.
	s = &State{
		blockMap:  make(map[BlockID]*blockNode),
		badBlocks: make(map[BlockID]struct{}),

		currentPath: make([]BlockID, 1),

		siacoinOutputs:        make(map[SiacoinOutputID]SiacoinOutput),
		fileContracts:         make(map[FileContractID]FileContract),
		siafundOutputs:        make(map[SiafundOutputID]SiafundOutput),
		delayedSiacoinOutputs: make(map[BlockHeight]map[SiacoinOutputID]SiacoinOutput),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := Block{
		Timestamp: genesisTime,
	}
	s.blockRoot = &blockNode{
		block:  genesisBlock,
		target: RootTarget,
		depth:  RootDepth,

		diffsGenerated: true,
	}
	s.blockMap[genesisBlock.ID()] = s.blockRoot

	// Fill out the consensus information for the genesis block.
	s.currentPath[0] = genesisBlock.ID()
	s.siacoinOutputs[genesisBlock.MinerPayoutID(0)] = SiacoinOutput{
		Value:      CalculateCoinbase(0),
		UnlockHash: ZeroUnlockHash,
	}
	s.siafundOutputs[SiafundOutputID{0}] = SiafundOutput{
		Value:           NewCurrency64(SiafundCount),
		UnlockHash:      fundUnlockHash,
		ClaimUnlockHash: claimUnlockHash,
	}

	return
}

// CreateGenesisState returns a State containing only the genesis block.
func CreateGenesisState() (s *State) {
	return createGenesisState(GenesisTimestamp, GenesisSiafundUnlockHash, GenesisClaimUnlockHash)
}

// RLock will readlock the state.
//
// TODO: Add a safety timer which will auto-unlock if the readlock is held for
// more than a second. (panic in debug mode)
func (s *State) RLock() {
	s.mu.RLock()
}

// RUnlock will readunlock the state.
//
// TODO: when the safety timer is added to RLock, add a timer disabler to
// RUnlock to prevent too many unlocks from being called.
func (s *State) RUnlock() {
	s.mu.RUnlock()
}
