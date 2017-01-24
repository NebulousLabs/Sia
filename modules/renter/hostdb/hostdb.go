// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

// TODO: There should be some mechanism for detecting if the hostdb cannot
// connect to the internet. If it cannot, hosts should not be penalized for
// appearing to be offline, because they may not actually be offline and it'll
// unfairly over-penalize the hosts with the highest uptime.
//
// Do this by adding a gateway and checking for non-local nodes.

// TODO: Need to distinguish between scans that were triggered by a fresh
// blockchain announcement and scans that were triggered by cycle selection
// (makes a difference in how the uptime stats should be counted)

// TODO: Proper upgrade for hostdb from prior persist. Also, need default
// settings for hosts that fail the first scan.

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
	// random.
	hostTree    *hosttree.HostTree

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

		scanPool:    make(chan *hostEntry, scanPoolSize),
	}

	// The host tree is used to manage hosts and query them at random.
	hdb.hostTree = hosttree.New(hdb.calculateHostWeight)

	// Load the prior persistence structures.
	err := hdb.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(hdb, hdb.lastChange)
	if err == modules.ErrInvalidConsensusChangeID {
		// Subscribe again using the new ID. This will cause a triggered scan
		// on all of the hosts, but that should be acceptable.
		hdb.lastChange = modules.ConsensusChangeBeginning
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

// ActiveHosts returns the hosts that can be randomly selected out of the
// hostdb, sorted by preference.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostDBEntry) {
	hdb.mu.RLock()
	numHosts := len(hdb.activeHosts)
	hdb.mu.RUnlock()

	// Get the hosts using RandomHosts so that they are in sorted order.
	sortedHosts, err := hdb.hostTree.SelectRandom(numHosts, nil)
	if err != nil {
		hdb.log.Severe("error selecting random hosts in ActiveHosts() call: ", err)
	}
	return sortedHosts
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostDBEntry) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	for _, entry := range hdb.allHosts {
		allHosts = append(allHosts, entry.HostDBEntry)
	}
	return
}

// AverageContractPrice returns the average price of a host.
func (hdb *HostDB) AverageContractPrice() types.Currency {
	// maybe a more sophisticated way of doing this
	var totalPrice types.Currency
	sampleSize := 32
	hosts, err := hdb.hostTree.SelectRandom(sampleSize, nil)
	if err != nil {
		hdb.log.Severe("error selecting random hosts in AverageContractPrice() call: ", err)
	}
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
func (hdb *HostDB) Host(addr modules.NetAddress) (modules.HostDBEntry, bool) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	entry, ok := hdb.allHosts[addr]
	if !ok || entry == nil {
		return modules.HostDBEntry{}, false
	}
	return entry.HostDBEntry, true
}

// RandomHosts implements the HostDB interface's RandomHosts() method. It takes
// a number of hosts to return, and a slice of netaddresses to ignore, and
// returns a slice of entries.
func (hdb *HostDB) RandomHosts(n int, excludeKeys []types.SiaPublicKey) []modules.HostDBEntry {
	hosts, err := hdb.hostTree.SelectRandom(n, excludeKeys)
	if err != nil {
		hdb.log.Debugln("error selecting random hosts from the tree: ", err)
	}
	return hosts
}
