// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

import (
	"errors"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
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
	log     logger
	persist persister
	sleeper sleeper

	// The hostTree is the root node of the tree that organizes hosts by
	// weight. The tree is necessary for selecting weighted hosts at
	// random. 'activeHosts' provides a lookup from hostname to the the
	// corresponding node, as the hostTree is unsorted. A host is active if
	// it is currently responding to queries about price and other
	// settings.
	hostTree    *hostNode
	activeHosts map[modules.NetAddress]*hostNode

	// allHosts is a simple list of all known hosts by their network address,
	// including hosts that are currently offline.
	allHosts map[modules.NetAddress]*hostEntry

	// the scanPool is a set of hosts that need to be scanned. There are a
	// handful of goroutines constantly waiting on the channel for hosts to
	// scan.
	scanPool chan *hostEntry

	blockHeight types.BlockHeight

	mu sync.RWMutex
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
	logger, err := newLogger(persistDir)
	if err != nil {
		return nil, err
	}

	// Create HostDB using production dependencies.
	return newHostDB(cs, stdDialer{}, stdSleeper{}, newPersist(persistDir), logger)
}

// newHostDB creates a HostDB using the provided dependencies. It loads the old
// persistence data, spawns the HostDB's scanning threads, and subscribes it to
// the consensusSet.
func newHostDB(cs consensusSet, d dialer, s sleeper, p persister, l logger) (*HostDB, error) {
	// Create the HostDB object.
	hdb := &HostDB{
		dialer:  d,
		sleeper: s,
		persist: p,
		log:     l,

		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),
	}

	// Load the prior persistance structures.
	err := hdb.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Begin listening to consensus and looking for hosts.
	for i := 0; i < scanningThreads; i++ {
		go hdb.threadedProbeHosts()
	}
	go hdb.threadedScan()

	err = cs.ConsensusSetPersistentSubscribe(hdb, modules.ConsensusChangeID{})
	if err != nil {
		return nil, errors.New("hostdb subscription failed: " + err.Error())
	}

	return hdb, nil
}
