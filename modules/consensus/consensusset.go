package consensus

// All changes to the consenuss set are made via diffs, specifically by calling
// a commitDiff function. This means that future modifications (such as
// replacing in-memory versions of the utxo set with on-disk versions of the
// utxo set) should be relatively easy to verify for correctness. Modifying the
// commitDiff functions will be sufficient.

import (
	"errors"
	"os"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrNilGateway = errors.New("cannot have a nil gateway as input")
)

// The ConsensusSet is the object responsible for tracking the current status
// of the blockchain. Broadly speaking, it is responsible for maintaining
// consensus.  It accepts blocks and constructs a blockchain, forking when
// necessary.
type ConsensusSet struct {
	// The gateway manages peer connections and keeps the consensus set
	// synchronized to the rest of the network.
	gateway modules.Gateway

	// The block root contains the genesis block.
	blockRoot processedBlock

	// The set database stores the consensus set on disk. The variables it
	// contains are siacoinOutputs, fileContracts, and siafundOutputs. They
	// keep track of the unspent outputs and active contracts present in the
	// current path. If an output is spent or a contract expires, it is removed
	// from the consensus set.
	//
	// It also holds delayedSiacoinOutputs, which are siacoin outputs that have
	// been created in a block, but are not allowed to be spent until a certain
	// height. When that height is reached, they are moved to the
	// siacoinOutputs map.
	//
	// The database also holds the file contract expirations.
	// FileContractExpirations is not actually a part of the consensus set, but
	// it is needed to get decent order notation on the file contract lookups.
	// It is a map of heights to maps of file contract ids. The other table is
	// needed because of file contract revisions - you need to have random
	// access lookups to file contracts for when revisions are submitted to the
	// blockchain.
	db *setDB

	// The siafundPool tracks the total number of siacoins that have been taxed
	// from file contracts. Unless a reorg occurs, the siafundPool should never
	// decrease.
	siafundPool types.Currency

	// Modules subscribed to the consensus set will receive an ordered list of
	// changes that occur to the consensus set, computed using the changeLog.
	changeLog   []changeEntry
	subscribers []modules.ConsensusSetSubscriber

	// dosBlocks keeps track of seen blocks. It is a "blacklist" of blocks
	// known to the expensive part of block validation.
	dosBlocks map[types.BlockID]struct{}

	// The entire consensus set is protected by a single mutex. While this
	// inhibits parallel lookups to parallel data, it reduces overall
	// complexity.
	mu *sync.RWMutex
}

// New returns a new ConsensusSet, containing at least the genesis block. If
// there is an existing block database present in saveDir, it will be loaded.
// Otherwise, a new database will be created.
func New(gateway modules.Gateway, saveDir string) (*ConsensusSet, error) {
	if gateway == nil {
		return nil, ErrNilGateway
	}

	// Create the genesis block.
	genesisBlock := types.Block{
		Timestamp: types.GenesisTimestamp,
		Transactions: []types.Transaction{
			{SiafundOutputs: types.GenesisSiafundAllocation},
		},
	}

	// Create the ConsensusSet object.
	cs := &ConsensusSet{
		gateway: gateway,

		blockRoot: processedBlock{
			Block:       genesisBlock,
			ChildTarget: types.RootTarget,
			Depth:       types.RootDepth,

			DiffsGenerated: true,
		},

		dosBlocks: make(map[types.BlockID]struct{}),

		mu: sync.New(modules.SafeMutexDelay, 1),
	}

	// Allocate the Siafund addresses by putting them all in a big transaction inside the genesis block
	for i, siafundOutput := range genesisBlock.Transactions[0].SiafundOutputs {
		sfid := genesisBlock.Transactions[0].SiafundOutputID(i)
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfid,
			SiafundOutput: siafundOutput,
		}
		cs.blockRoot.SiafundOutputDiffs = append(cs.blockRoot.SiafundOutputDiffs, sfod)
	}

	// Create the consensus directory.
	err := os.MkdirAll(saveDir, 0700)
	if err != nil {
		return nil, err
	}

	// Try to load an existing database from disk.
	err = cs.load(saveDir)
	if err != nil {
		return nil, err
	}

	// Send the genesis block to subscribers.
	cs.updateSubscribers(nil, []*processedBlock{&cs.blockRoot})

	// Send any blocks that were loaded from disk to subscribers.
	cs.loadDiffs()

	// Register RPCs
	gateway.RegisterRPC("SendBlocks", cs.sendBlocks)
	gateway.RegisterRPC("RelayBlock", cs.RelayBlock)
	gateway.RegisterConnectCall("SendBlocks", cs.receiveBlocks)

	return cs, nil
}

// Close safely closes the block database.
func (cs *ConsensusSet) Close() error {
	lockID := cs.mu.Lock()
	defer cs.mu.Unlock(lockID)
	cs.db.open = false
	return cs.db.Close()
}
