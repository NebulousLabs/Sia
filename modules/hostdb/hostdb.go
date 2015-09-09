// package hostdb provides a HostDB object that implements the modules.HostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

// hostdb.go defines the HostDB type and New for the funtion.

import (
	"errors"
	"log"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
)

const (
	// scanPoolSize sets the buffer size of the channel that holds hosts which
	// need to be scanned. A thread pool pulls from the scan pool to query
	// hosts that are due for an update.
	scanPoolSize = 1000
)

var (
	errNilGateway      = errors.New("gateway cannot be nil")
	errNilConsensusSet = errors.New("consensus set cannot be nil")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	consensusSet    modules.ConsensusSet
	gateway         modules.Gateway
	consensusHeight int

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

	persistDir string
	log        *log.Logger
	mu         *sync.RWMutex
}

// New creates and starts up a hostdb. The hostdb that gets returned will not
// have finished scanning the network or blockchain.
func New(cs modules.ConsensusSet, g modules.Gateway, persistDir string) (*HostDB, error) {
	// Check for nil dependencies.
	if cs == nil {
		return nil, errNilConsensusSet
	}
	if g == nil {
		return nil, errNilGateway
	}

	// Build an empty hostdb.
	hdb := &HostDB{
		consensusSet: cs,
		gateway:      g,

		activeHosts: make(map[modules.NetAddress]*hostNode),

		allHosts: make(map[modules.NetAddress]*hostEntry),

		scanPool: make(chan *hostEntry, scanPoolSize),

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	err := hdb.initPersist()
	if err != nil {
		return nil, err
	}

	// Begin listening to consensus and looking for hosts.
	for i := 0; i < scanningThreads; i++ {
		go hdb.threadedProbeHosts()
	}
	go hdb.threadedScan()
	cs.ConsensusSetSubscribe(hdb)
	return hdb, nil
}
