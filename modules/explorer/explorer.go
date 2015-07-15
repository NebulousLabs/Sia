package explorer

import (
	"errors"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// The explorer module provides a glimpse into what the Sia network
// currently looks like.

// Basic structure to store the blockchain. Metadata may also be
// stored here in the future
type Explorer struct {
	// db is the currently opened database. the explorerDB passes
	// through to a persist.BoltDatabase, which passes through to
	// a bolt.db
	db *explorerDB

	// CurBlock is the current highest block on the blockchain,
	// kept update via a subscription to consensus
	currentBlock types.Block

	// Stored to differentiate the special case of the genesis
	// block when recieving updates from consensus, to avoid
	// constant queries to consensus.
	genesisBlockID types.BlockID

	// Used for caching the current blockchain height
	blockchainHeight types.BlockHeight

	// Used to  store the time blocks were seen.  Sholud only keep
	// track of the most recent 144 blocks. Its size should always
	// be types.MaturityDelay. SeenTimes  keeps track of that many
	//  times,  and should  be  indexed  with (blockchainheight  %
	// len(seenTimes))
	seenTimes []time.Time

	// stores the time that the blockexplorer started so that new
	// seen blocks can be compared against it.
	startTime time.Time

	// currencySent keeps track of how much currency has been
	// i.e. sending siacoin to somebody else
	currencySent types.Currency

	// activeContracts and totalContracts hold the current number
	// of file contracts now in effect and that ever have been,
	// respectively
	activeContracts uint64
	totalContracts  uint64

	// activeContracts and totalContracts hold the current sum
	// cost of the file contracts now in effect and that ever have
	// been, respectively
	activeContractCost types.Currency
	totalContractCost  types.Currency

	// activeContractSize and totalCotractSize keep a running
	// count of the amount of bytes that have been sent over file
	// contracts
	activeContractSize uint64
	totalContractSize  uint64

	// Keep a reference to the consensus for queries
	cs modules.ConsensusSet

	// Subscriptions currently contain no data, but serve to
	// notify other modules when changes occur
	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New creates the internal data structures, and subscribes to
// consensus for changes to the blockchain
func New(cs modules.ConsensusSet, persistDir string) (e *Explorer, err error) {
	// Check that input modules are non-nil
	if cs == nil {
		err = errors.New("Blockchain explorer cannot use a nil ConsensusSet")
		return
	}

	// Make the persist directory
	err = os.MkdirAll(persistDir, 0700)
	if err != nil {
		return
	}

	// Initilize the database
	db, err := openDB(persistDir + "/blocks.db")
	if err != nil {
		return nil, err
	}

	// Initilize the module state
	e = &Explorer{
		db:                 db,
		currentBlock:       cs.GenesisBlock(),
		genesisBlockID:     cs.GenesisBlock().ID(),
		blockchainHeight:   0,
		seenTimes:          make([]time.Time, types.MaturityDelay+1),
		startTime:          time.Now(),
		currencySent:       types.NewCurrency64(0),
		activeContractCost: types.NewCurrency64(0),
		totalContractCost:  types.NewCurrency64(0),
		cs:                 cs,
		mu:                 sync.New(modules.SafeMutexDelay, 1),
	}

	cs.ConsensusSetSubscribe(e)

	return
}

func (e *Explorer) Close() error {
	return e.db.CloseDatabase()
}
