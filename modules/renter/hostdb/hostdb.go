// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb/hosttree"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/threadgroup"
)

var (
	// ErrInitialScanIncomplete is returned whenever an operation is not
	// allowed to be executed before the initial host scan has finished.
	ErrInitialScanIncomplete = errors.New("initial hostdb scan is not yet completed")
	errNilCS                 = errors.New("cannot create hostdb with nil consensus set")
	errNilGateway            = errors.New("cannot create hostdb with nil gateway")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	// dependencies
	cs         modules.ConsensusSet
	deps       modules.Dependencies
	gateway    modules.Gateway
	log        *persist.Logger
	mu         sync.RWMutex
	persistDir string
	tg         threadgroup.ThreadGroup

	// The hostTree is the root node of the tree that organizes hosts by
	// weight. The tree is necessary for selecting weighted hosts at
	// random.
	hostTree *hosttree.HostTree

	// the scanPool is a set of hosts that need to be scanned. There are a
	// handful of goroutines constantly waiting on the channel for hosts to
	// scan. The scan map is used to prevent duplicates from entering the scan
	// pool.
	initialScanComplete  bool
	initialScanLatencies []time.Duration
	scanList             []modules.HostDBEntry
	scanMap              map[string]struct{}
	scanWait             bool
	scanningThreads      int

	blockHeight types.BlockHeight
	lastChange  modules.ConsensusChangeID
}

// New returns a new HostDB.
func New(g modules.Gateway, cs modules.ConsensusSet, persistDir string) (*HostDB, error) {
	// Check for nil inputs.
	if g == nil {
		return nil, errNilGateway
	}
	if cs == nil {
		return nil, errNilCS
	}
	// Create HostDB using production dependencies.
	return NewCustomHostDB(g, cs, persistDir, modules.ProdDependencies)
}

// NewCustomHostDB creates a HostDB using the provided dependencies. It loads the old
// persistence data, spawns the HostDB's scanning threads, and subscribes it to
// the consensusSet.
func NewCustomHostDB(g modules.Gateway, cs modules.ConsensusSet, persistDir string, deps modules.Dependencies) (*HostDB, error) {
	// Create the HostDB object.
	hdb := &HostDB{
		cs:         cs,
		deps:       deps,
		gateway:    g,
		persistDir: persistDir,

		scanMap: make(map[string]struct{}),
	}

	// Create the persist directory if it does not yet exist.
	err := os.MkdirAll(persistDir, 0700)
	if err != nil {
		return nil, err
	}

	// Create the logger.
	logger, err := persist.NewFileLogger(filepath.Join(persistDir, "hostdb.log"))
	if err != nil {
		return nil, err
	}
	hdb.log = logger
	err = hdb.tg.AfterStop(func() error {
		if err := hdb.log.Close(); err != nil {
			// Resort to println as the logger is in an uncertain state.
			fmt.Println("Failed to close the hostdb logger:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The host tree is used to manage hosts and query them at random.
	hdb.hostTree = hosttree.New(hdb.calculateHostWeight)

	// Load the prior persistence structures.
	hdb.mu.Lock()
	err = hdb.load()
	hdb.mu.Unlock()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	err = hdb.tg.AfterStop(func() error {
		hdb.mu.Lock()
		err := hdb.saveSync()
		hdb.mu.Unlock()
		if err != nil {
			hdb.log.Println("Unable to save the hostdb:", err)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Loading is complete, establish the save loop.
	go hdb.threadedSaveLoop()

	// Don't perform the remaining startup in the presence of a quitAfterLoad
	// disruption.
	if hdb.deps.Disrupt("quitAfterLoad") {
		return hdb, nil
	}

	// COMPATv1.1.0
	//
	// If the block height has loaded as zero, the most recent consensus change
	// needs to be set to perform a full rescan. This will also help the hostdb
	// to pick up any hosts that it has incorrectly dropped in the past.
	hdb.mu.Lock()
	if hdb.blockHeight == 0 {
		hdb.lastChange = modules.ConsensusChangeBeginning
	}
	hdb.mu.Unlock()

	err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange, hdb.tg.StopChan())
	if err == modules.ErrInvalidConsensusChangeID {
		// Subscribe again using the new ID. This will cause a triggered scan
		// on all of the hosts, but that should be acceptable.
		hdb.mu.Lock()
		hdb.blockHeight = 0
		hdb.lastChange = modules.ConsensusChangeBeginning
		hdb.mu.Unlock()
		err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange, hdb.tg.StopChan())
	}
	if err != nil {
		return nil, errors.New("hostdb subscription failed: " + err.Error())
	}
	err = hdb.tg.OnStop(func() error {
		cs.Unsubscribe(hdb)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Spawn the scan loop during production, but allow it to be disrupted
	// during testing. Primary reason is so that we can fill the hostdb with
	// fake hosts and not have them marked as offline as the scanloop operates.
	if !hdb.deps.Disrupt("disableScanLoop") {
		go hdb.threadedScan()
	} else {
		hdb.initialScanComplete = true
	}

	return hdb, nil
}

// ActiveHosts returns a list of hosts that are currently online, sorted by
// weight.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostDBEntry) {
	allHosts := hdb.hostTree.All()
	for _, entry := range allHosts {
		if len(entry.ScanHistory) == 0 {
			continue
		}
		if !entry.ScanHistory[len(entry.ScanHistory)-1].Success {
			continue
		}
		if !entry.AcceptingContracts {
			continue
		}
		activeHosts = append(activeHosts, entry)
	}
	return activeHosts
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostDBEntry) {
	return hdb.hostTree.All()
}

// AverageContractPrice returns the average price of a host.
func (hdb *HostDB) AverageContractPrice() (totalPrice types.Currency) {
	sampleSize := 32
	hosts := hdb.hostTree.SelectRandom(sampleSize, nil)
	if len(hosts) == 0 {
		return totalPrice
	}
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.ContractPrice)
	}
	return totalPrice.Div64(uint64(len(hosts)))
}

// Close closes the hostdb, terminating its scanning threads
func (hdb *HostDB) Close() error {
	return hdb.tg.Stop()
}

// Host returns the HostSettings associated with the specified NetAddress. If
// no matching host is found, Host returns false.
func (hdb *HostDB) Host(spk types.SiaPublicKey) (modules.HostDBEntry, bool) {
	host, exists := hdb.hostTree.Select(spk)
	if !exists {
		return host, exists
	}
	hdb.mu.RLock()
	updateHostHistoricInteractions(&host, hdb.blockHeight)
	hdb.mu.RUnlock()
	return host, exists
}

// InitialScanComplete returns a boolean indicating if the initial scan of the
// hostdb is completed.
func (hdb *HostDB) InitialScanComplete() (complete bool, err error) {
	if err = hdb.tg.Add(); err != nil {
		return
	}
	defer hdb.tg.Done()
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	complete = hdb.initialScanComplete
	return
}

// RandomHosts implements the HostDB interface's RandomHosts() method. It takes
// a number of hosts to return, and a slice of netaddresses to ignore, and
// returns a slice of entries.
func (hdb *HostDB) RandomHosts(n int, excludeKeys []types.SiaPublicKey) ([]modules.HostDBEntry, error) {
	hdb.mu.RLock()
	initialScanComplete := hdb.initialScanComplete
	hdb.mu.RUnlock()
	if !initialScanComplete {
		return []modules.HostDBEntry{}, ErrInitialScanIncomplete
	}
	return hdb.hostTree.SelectRandom(n, excludeKeys), nil
}
