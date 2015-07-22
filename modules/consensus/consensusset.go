package consensus

// All changes to the consenuss set are made via diffs, specifically by calling
// a commitDiff function. This means that future modifications (such as
// replacing in-memory versions of the utxo set with on-disk versions of the
// utxo set) should be relatively easy to verify for correctness. Modifying the
// commitDiff functions will be sufficient.

import (
	"errors"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// fullVerification indicates that a block should be fully verified when
	// being loaded from disk.
	fullVerification verificationRigor = 0

	// partialVerification indicates that transaction verification can be
	// skipped. Transaction verification is computationally intensive, and
	// skipping such a step noticably increases speed when loading many blocks
	// at once. Usually, partialVerification is used when loading blocks from
	// disk.
	partialVerification verificationRigor = 1
)

var (
	ErrNilGateway = errors.New("cannot have a nil gateway as input")
)

// verificationRigor is a type indicating the intensity of verification that
// should be using while accepting a block. For blocks that come from trusted
// sources, the computationally expensive steps can be skipped.
type verificationRigor byte

// The ConsensusSet is the object responsible for tracking the current status
// of the blockchain. Broadly speaking, it is responsible for maintaining
// consensus.  It accepts blocks and constructs a blockchain, forking when
// necessary.
type ConsensusSet struct {
	// verificationRigor is a flag that tells the state whether or not to do
	// transaction verification while accepting a block. This should help speed
	// up loading blocks from memory.
	verificationRigor verificationRigor

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

	// fileContractExpirations is not actually a part of the consensus set, but
	// it is needed to get decent order notation on the file contract lookups.
	// It is a map of heights to maps of file contract ids. The other table is
	// needed because of file contract revisions - you need to have random
	// access lookups to file contracts for when revisions are submitted to the
	// blockchain.
	fileContractExpirations map[types.BlockHeight]map[types.FileContractID]struct{}

	// Modules subscribed to the consensus set will receive an ordered list of
	// changes that occur to the consensus set, computed using the changeLog.
	changeLog   []changeEntry
	subscribers []modules.ConsensusSetSubscriber

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

// New returns a new ConsensusSet, containing at least the genesis block. If
// there is an existing block database present in saveDir, it will be loaded.
// Otherwise, a new database will be created.
func New(gateway modules.Gateway, saveDir string) (*ConsensusSet, error) {
	if gateway == nil {
		return nil, ErrNilGateway
	}

	// Create the ConsensusSet object.
	cs := &ConsensusSet{
		blockMap:  make(map[types.BlockID]*blockNode),
		dosBlocks: make(map[types.BlockID]struct{}),

		currentPath: make([]types.BlockID, 1),

		siacoinOutputs:        make(map[types.SiacoinOutputID]types.SiacoinOutput),
		fileContracts:         make(map[types.FileContractID]types.FileContract),
		siafundOutputs:        make(map[types.SiafundOutputID]types.SiafundOutput),
		delayedSiacoinOutputs: make(map[types.BlockHeight]map[types.SiacoinOutputID]types.SiacoinOutput),

		fileContractExpirations: make(map[types.BlockHeight]map[types.FileContractID]struct{}),

		gateway: gateway,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := types.Block{
		Timestamp: types.GenesisTimestamp,
		Transactions: []types.Transaction{
			{SiafundOutputs: types.GenesisSiafundAllocation},
		},
	}
	cs.blockRoot = &blockNode{
		block:       genesisBlock,
		childTarget: types.RootTarget,
		depth:       types.RootDepth,

		diffsGenerated: true,
	}
	cs.blockMap[genesisBlock.ID()] = cs.blockRoot

	// Fill out the consensus information for the genesis block.
	cs.currentPath[0] = genesisBlock.ID()
	cs.siacoinOutputs[genesisBlock.MinerPayoutID(0)] = types.SiacoinOutput{
		Value:      types.CalculateCoinbase(0),
		UnlockHash: types.ZeroUnlockHash,
	}

	// Allocate the Siafund addresses by putting them all in a big transaction
	// and applying the diffs.
	for i, siafundOutput := range genesisBlock.Transactions[0].SiafundOutputs {
		sfid := genesisBlock.Transactions[0].SiafundOutputID(i)
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfid,
			SiafundOutput: siafundOutput,
		}
		cs.commitSiafundOutputDiff(sfod, modules.DiffApply)
		cs.blockRoot.siafundOutputDiffs = append(cs.blockRoot.siafundOutputDiffs, sfod)
	}
	if build.DEBUG {
		cs.blockRoot.consensusSetHash = cs.consensusSetHash()
	}

	// Send out genesis block update.
	cs.updateSubscribers(nil, []*blockNode{cs.blockRoot})

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

	return cs, nil
}

// Close safely closes the block database.
func (cs *ConsensusSet) Close() error {
	return cs.db.Close()
}
