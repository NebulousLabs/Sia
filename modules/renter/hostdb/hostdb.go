// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

// TODO: There should be some mechanism that detects if the number of active
// hosts is low. Then either the user can be informed, or the hostdb can start
// scanning hosts that have been offline for a while and are no longer
// prioritized by the scan loop.

// TODO: There should be some mechanism for detecting if the hostdb cannot
// connect to the internet. If it cannot, hosts should not be penalized for
// appearing to be offline, because they may not actually be offline and it'll
// unfairly over-penalize the hosts with the highest uptime.

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/hostdb/hosttree"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// scanPoolSize sets the buffer size of the channel that holds hosts which
	// need to be scanned. A thread pool pulls from the scan pool to query
	// hosts that are due for an update.
	scanPoolSize = 1000
)

var (
	errNilCS = errors.New("cannot create hostdb with nil consensus set")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	// dependencies
	dialer  dialer
	log     *persist.Logger
	mu      sync.RWMutex
	persist persister
	sleeper sleeper
	tg      siasync.ThreadGroup

	// The hostTree is the root node of the tree that organizes hosts by
	// weight. The tree is necessary for selecting weighted hosts at
	// random. 'activeHosts' provides a lookup from hostname to the the
	// corresponding node, as the hostTree is unsorted. A host is active if
	// it is currently responding to queries about price and other
	// settings.
	hostTree    *hosttree.HostTree
	activeHosts map[modules.NetAddress]*hostEntry

	// allHosts is a simple list of all known hosts by their network address,
	// including hosts that are currently offline.
	allHosts map[modules.NetAddress]*hostEntry

	// the scanPool is a set of hosts that need to be scanned. There are a
	// handful of goroutines constantly waiting on the channel for hosts to
	// scan.
	scanList []*hostEntry
	scanPool chan *hostEntry
	scanWait bool

	blockHeight types.BlockHeight
	lastChange  modules.ConsensusChangeID
}

// New returns a new HostDB.
func New(cs consensusSet, persistDir string) (*HostDB, error) {
	// Check for nil inputs.
	if cs == nil {
		return nil, errNilCS
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

	// Create HostDB using production dependencies.
	return newHostDB(cs, stdDialer{}, stdSleeper{}, newPersist(persistDir), logger)
}

// newHostDB creates a HostDB using the provided dependencies. It loads the old
// persistence data, spawns the HostDB's scanning threads, and subscribes it to
// the consensusSet.
func newHostDB(cs consensusSet, d dialer, s sleeper, p persister, l *persist.Logger) (*HostDB, error) {
	// Create the HostDB object.
	hdb := &HostDB{
		dialer:  d,
		sleeper: s,
		persist: p,
		log:     l,

		// TODO: should index by pubkey, not ip
		activeHosts: make(map[modules.NetAddress]*hostEntry),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),
	}

	hdb.hostTree = hosttree.New(hdb.calculateHostWeight())

	// Load the prior persistence structures.
	err := hdb.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange)
	if err == modules.ErrInvalidConsensusChangeID {
		hdb.lastChange = modules.ConsensusChangeBeginning
		// clear the host sets
		hdb.activeHosts = make(map[modules.NetAddress]*hostEntry)
		hdb.allHosts = make(map[modules.NetAddress]*hostEntry)
		// subscribe again using the new ID
		err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange)
	}
	if err != nil {
		return nil, errors.New("hostdb subscription failed: " + err.Error())
	}

	// Spin up the host scanning processes.
	for i := 0; i < scanningThreads; i++ {
		go hdb.threadedProbeHosts()
	}
	go hdb.threadedScan()

	return hdb, nil
}

// Close closes the hostdb, terminating its scanning threads
func (hdb *HostDB) Close() error {
	return hdb.tg.Stop()
}

func (hdb *HostDB) RandomHosts(n int, exclude []modules.NetAddress) []modules.HostDBEntry {
	// Convert exclusion netaddresses to public keys
	var excludeKeys []types.SiaPublicKey
	for _, addr := range exclude {
		entry, exists := hdb.activeHosts[addr]
		if exists {
			excludeKeys = append(excludeKeys, entry.HostDBEntry.PublicKey)
		}
	}

	hosts, err := hdb.hostTree.SelectRandom(n, excludeKeys)
	if err != nil {
		// TODO: handle this err
	}
	return hosts
}
