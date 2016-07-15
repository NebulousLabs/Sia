package consensus

// All changes to the consenuss set are made via diffs, specifically by calling
// a commitDiff function. This means that future modifications (such as
// replacing in-memory versions of the utxo set with on-disk versions of the
// utxo set) should be relatively easy to verify for correctness. Modifying the
// commitDiff functions will be sufficient.

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/demotemutex"
)

var (
	errNilGateway = errors.New("cannot have a nil gateway as input")
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

	// Subscribers to the consensus set will receive a changelog every time
	// there is an update to the consensus set. At initialization, they receive
	// all changes that they are missing.
	//
	// Memory: A consensus set typically has fewer than 10 subscribers, and
	// subscription typically happens entirely at startup. This slice is
	// unlikely to grow beyond 1kb, and cannot by manipulated by an attacker as
	// the function of adding a subscriber should not be exposed.
	subscribers []modules.ConsensusSetSubscriber

	// dosBlocks are blocks that are invalid, but the invalidity is only
	// discoverable during an expensive step of validation. These blocks are
	// recorded to eliminate a DoS vector where an expensive-to-validate block
	// is submitted to the consensus set repeatedly.
	//
	// TODO: dosBlocks needs to be moved into the database, and if there's some
	// reason it can't be in THE database, it should be in a separate database.
	// dosBlocks is an unbounded map that an attacker can manipulate, though
	// iirc manipulations are expensive, to the tune of creating a blockchain
	// PoW per DoS block (though the attacker could conceivably build off of
	// the genesis block, meaning the PoW is not very expensive.
	dosBlocks map[types.BlockID]struct{}

	// checkingConsistency is a bool indicating whether or not a consistency
	// check is in progress. The consistency check logic call itself, resulting
	// in infinite loops. This bool prevents that while still allowing for full
	// granularity consistency checks. Previously, consistency checks were only
	// performed after a full reorg, but now they are performed after every
	// block.
	checkingConsistency bool

	// synced is true if initial blockchain download has finished. It indicates
	// whether the consensus set is synced with the network.
	synced bool

	// Interfaces to abstract the dependencies of the ConsensusSet.
	marshaler       encoding.GenericMarshaler
	blockRuleHelper blockRuleHelper
	blockValidator  blockValidator

	// Utilities
	db         *persist.BoltDatabase
	log        *persist.Logger
	mu         demotemutex.DemoteMutex
	persistDir string
	tg         sync.ThreadGroup
}

// New returns a new ConsensusSet, containing at least the genesis block. If
// there is an existing block database present in the persist directory, it
// will be loaded.
func New(gateway modules.Gateway, persistDir string) (*ConsensusSet, error) {
	// Check for nil dependencies.
	if gateway == nil {
		return nil, errNilGateway
	}

	// Create the ConsensusSet object.
	cs := &ConsensusSet{
		gateway: gateway,

		blockRoot: processedBlock{
			Block:       types.GenesisBlock,
			ChildTarget: types.RootTarget,
			Depth:       types.RootDepth,

			DiffsGenerated: true,
		},

		dosBlocks: make(map[types.BlockID]struct{}),

		marshaler:       encoding.StdGenericMarshaler{},
		blockRuleHelper: stdBlockRuleHelper{},
		blockValidator:  NewBlockValidator(),

		persistDir: persistDir,
	}

	// Create the diffs for the genesis siafund outputs.
	for i, siafundOutput := range types.GenesisBlock.Transactions[0].SiafundOutputs {
		sfid := types.GenesisBlock.Transactions[0].SiafundOutputID(uint64(i))
		sfod := modules.SiafundOutputDiff{
			Direction:     modules.DiffApply,
			ID:            sfid,
			SiafundOutput: siafundOutput,
		}
		cs.blockRoot.SiafundOutputDiffs = append(cs.blockRoot.SiafundOutputDiffs, sfod)
	}

	// Initialize the consensus persistence structures.
	err := cs.initPersist()
	if err != nil {
		return nil, err
	}

	go func() {
		// Sync with the network. Don't sync if we are testing because typically we
		// don't have any mock peers to synchronize with in testing.
		// TODO: figure out a better way to conditionally do IBD. Otherwise this block will never be tested.
		if build.Release != "testing" {
			cs.threadedInitialBlockchainDownload()
		}

		// Register RPCs
		gateway.RegisterRPC("SendBlocks", cs.rpcSendBlocks)
		gateway.RegisterRPC("RelayBlock", cs.rpcRelayBlock) // COMPATv0.5.1
		gateway.RegisterRPC("RelayHeader", cs.rpcRelayHeader)
		gateway.RegisterRPC("SendBlk", cs.rpcSendBlk)
		gateway.RegisterConnectCall("SendBlocks", cs.threadedReceiveBlocks)
		cs.tg.OnStop(func() {
			cs.mu.Lock()
			defer cs.mu.Unlock()

			cs.gateway.UnregisterRPC("SendBlocks")
			cs.gateway.UnregisterRPC("RelayBlock")
			cs.gateway.UnregisterRPC("RelayHeader")
			cs.gateway.UnregisterRPC("SendBlk")
			cs.gateway.UnregisterConnectCall("SendBlocks")
		})

		// Mark that we are synced with the network.
		cs.mu.Lock()
		cs.synced = true
		cs.mu.Unlock()
	}()

	return cs, nil
}

// BlockAtHeight returns the block at a given height.
func (cs *ConsensusSet) BlockAtHeight(height types.BlockHeight) (block types.Block, exists bool) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		id, err := getPath(tx, height)
		if err != nil {
			return err
		}
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		block = pb.Block
		exists = true
		return nil
	})
	return block, exists
}

// ChildTarget returns the target for the child of a block.
func (cs *ConsensusSet) ChildTarget(id types.BlockID) (target types.Target, exists bool) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		target = pb.ChildTarget
		exists = true
		return nil
	})
	return target, exists
}

// Close safely closes the block database.
func (cs *ConsensusSet) Close() error {
	err := cs.tg.Stop()
	if err != nil {
		return err
	}

	var errs []error
	if err := cs.db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("db.Close failed: %v", err))
	}
	if err := cs.log.Close(); err != nil {
		errs = append(errs, fmt.Errorf("log.Close failed: %v", err))
	}
	return build.JoinErrors(errs, "; ")
}

// CurrentBlock returns the latest block in the heaviest known blockchain.
func (cs *ConsensusSet) CurrentBlock() (block types.Block) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb := currentProcessedBlock(tx)
		block = pb.Block
		return nil
	})
	return block
}

// Flush will block until the consensus set has finished all in-progress
// routines.
func (cs *ConsensusSet) Flush() error {
	return cs.tg.Flush()
}

// Height returns the height of the consensus set.
func (cs *ConsensusSet) Height() (height types.BlockHeight) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		height = blockHeight(tx)
		return nil
	})
	return height
}

// InCurrentPath returns true if the block presented is in the current path,
// false otherwise.
func (cs *ConsensusSet) InCurrentPath(id types.BlockID) (inPath bool) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			inPath = false
			return nil
		}
		pathID, err := getPath(tx, pb.Height)
		if err != nil {
			inPath = false
			return nil
		}
		inPath = pathID == id
		return nil
	})
	return inPath
}

// MinimumValidChildTimestamp returns the earliest timestamp that the next block
// can have in order for it to be considered valid.
func (cs *ConsensusSet) MinimumValidChildTimestamp(id types.BlockID) (timestamp types.Timestamp, exists bool) {
	// Error is not checked because it does not matter.
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		timestamp = cs.blockRuleHelper.minimumValidChildTimestamp(tx.Bucket(BlockMap), pb)
		exists = true
		return nil
	})
	return timestamp, exists
}

// StorageProofSegment returns the segment to be used in the storage proof for
// a given file contract.
func (cs *ConsensusSet) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		index, err = storageProofSegment(tx, fcid)
		return nil
	})
	return index, err
}
