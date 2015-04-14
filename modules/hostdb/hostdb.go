// package hostdb provides a HostDB object that implements the modules.HostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

// hostdb.go defines the HostDB type and New for the funtion. The functions for
// inserting, removing, and querying hosts in the database were put in this
// file for lack of a better place to put them.

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
)

const (
	// Hosts will not be removed if there are fewer than this many hosts.
	// Eventually, this number should be in the low thousands.
	MinHostThreshold = 5
)

var (
	ErrNilGateway = errors.New("gateway cannot be nil")
	ErrNilState   = errors.New("consensus set cannot be nil")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	consensusSet *consensus.State
	gateway      modules.Gateway

	// The hostTree is the root node of the tree that organizes hosts by
	// weight. The tree is necessary for selecting weighted hosts at
	// random. 'activeHosts' provides a lookup from hostname to the the
	// corresponding node, as the hostTree is unsorted. A host is active if
	// it is currently responding to queries about price and other
	// settings.
	hostTree    *hostNode
	activeHosts map[modules.NetAddress]*hostNode

	//  allHosts is a simple list of all known hosts by their network
	//  address, including hosts that are currently offline.
	allHosts map[modules.NetAddress]*modules.HostEntry

	mu *sync.RWMutex
}

// New returns an empty HostDatabase.
func New(cs *consensus.State, g modules.Gateway) (hdb *HostDB, err error) {
	if cs == nil {
		err = ErrNilState
		return
	}
	if g == nil {
		err = ErrNilGateway
		return
	}

	hdb = &HostDB{
		consensusSet: cs,
		gateway:      g,

		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*modules.HostEntry),

		mu: sync.New(1*time.Second, 0),
	}

	cs.ConsensusSetSubscribe(hdb)
	go hdb.threadedScan()

	return
}

// ActiveHosts returns the hosts that can be randomly selected out of the
// hostdb.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostEntry) {
	id := hdb.mu.RLock()
	defer hdb.mu.RUnlock(id)

	for _, node := range hdb.activeHosts {
		activeHosts = append(activeHosts, node.hostEntry)
	}
	return
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostEntry) {
	id := hdb.mu.RLock()
	defer hdb.mu.RUnlock(id)

	for _, entry := range hdb.allHosts {
		allHosts = append(allHosts, *entry)
	}
	return
}

// InsertHost inserts a host entry into the database.
func (hdb *HostDB) InsertHost(entry modules.HostEntry) error {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	hdb.insertHost(entry)
	return nil
}

// RemoveHost removes a host from the database.
func (hdb *HostDB) RemoveHost(addr modules.NetAddress) error {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	return hdb.removeHost(addr)
}
