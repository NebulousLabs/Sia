package consensus

// All changes to the consenuss set are made via diffs, specifically by calling
// a commitDiff function. This means that future modifications (such as
// replacing in-memory versions of the utxo set with on-disk versions of the
// utxo set) should be relatively easy to verify for correctness. Modifying the
// commitDiff functions will be sufficient.

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
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

	// The db is a database holding the current consensus set.
	db *persist.BoltDatabase

	// Modules subscribed to the consensus set will receive an ordered list of
	// changes that occur to the consensus set, computed using the changeLog.
	changeLog         []changeEntry
	subscribers       []modules.ConsensusSetSubscriber
	digestSubscribers []modules.ConsensusSetDigestSubscriber

	// dosBlocks keeps track of seen blocks. It is a "blacklist" of blocks
	// known to the expensive part of block validation.
	dosBlocks map[types.BlockID]struct{}

	// checkingConsistency is a bool indicating whether or not a consistency
	// check is in progress. The consistency check logic call itself, resulting
	// in infinite loops. This bool prevents that while still allowing for full
	// granularity consistency checks. Previously, consistency checks were only
	// performed after a full reorg, but now they are performed after every
	// block.
	checkingConsistency bool

	// clock is a Clock interface that indicates the current system time.
	clock types.Clock

	// marshaler encodes and decodes between objects and byte slices.
	marshaler encoding.GenericMarshaler

	persistDir string
	mu         demotemutex.DemoteMutex
}

// New returns a new ConsensusSet, containing at least the genesis block. If
// there is an existing block database present in the persist directory, it
// will be loaded.
func New(gateway modules.Gateway, persistDir string) (*ConsensusSet, error) {
	// Check for nil dependencies.
	if gateway == nil {
		return nil, errNilGateway
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

		clock: types.StdClock{},

		marshaler: encoding.StdGenericMarshaler{},

		persistDir: persistDir,
	}

	// Create the diffs for the genesis siafund outputs.
	for i, siafundOutput := range genesisBlock.Transactions[0].SiafundOutputs {
		sfid := genesisBlock.Transactions[0].SiafundOutputID(uint64(i))
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

	// Register RPCs
	gateway.RegisterRPC("SendBlocks", cs.sendBlocks)
	gateway.RegisterRPC("RelayBlock", cs.relayBlock)
	gateway.RegisterConnectCall("SendBlocks", cs.threadedReceiveBlocks)
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
	return
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
	return
}

// Close safely closes the block database.
func (cs *ConsensusSet) Close() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.db.Close()
}

// EarliestChildTimestamp returns the earliest timestamp that the next block can
// have in order for it to be considered valid.
func (cs *ConsensusSet) EarliestChildTimestamp(id types.BlockID) (timestamp types.Timestamp, exists bool) {
	// Error is not checked because it does not matter.
	_ = cs.db.View(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return err
		}
		timestamp = earliestChildTimestamp(tx.Bucket(BlockMap), pb)
		exists = true
		return nil
	})
	return
}

// GenesisBlock returns the genesis block.
func (cs *ConsensusSet) GenesisBlock() types.Block {
	return cs.blockRoot.Block
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
	return
}

// StorageProofSegment returns the segment to be used in the storage proof for
// a given file contract.
func (cs *ConsensusSet) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	_ = cs.db.View(func(tx *bolt.Tx) error {
		index, err = storageProofSegment(tx, fcid)
		return nil
	})
	return
}
