// Package hostdb provides a HostDB object that implements the renter.hostDB
// interface. The blockchain is scanned for host announcements and hosts that
// are found get added to the host database. The database continually scans the
// set of hosts it has found and updates who is online.
package hostdb

import (
	"errors"
	"log"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
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
	errNilCS     = errors.New("cannot create hostdb with nil consensus set")
	errNilWallet = errors.New("cannot create hostdb with nil wallet")
	errNilTpool  = errors.New("cannot create hostdb with nil transaction pool")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	// dependencies
	wallet  hdbWallet
	tpool   hdbTransactionPool
	dialer  hdbDialer
	sleeper hdbSleeper
	persist hdbPersister

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

	blockHeight   types.BlockHeight
	contracts     map[types.FileContractID]hostContract
	cachedAddress types.UnlockHash // to prevent excessive address creation

	log *log.Logger
	mu  sync.RWMutex
}

// a hostContract includes the original contract made with a host, along with
// the most recent revision.
type hostContract struct {
	IP              modules.NetAddress
	ID              types.FileContractID
	FileContract    types.FileContract
	LastRevision    types.FileContractRevision
	LastRevisionTxn types.Transaction
	SecretKey       crypto.SecretKey
}

// New creates and starts up a hostdb. The hostdb that gets returned will not
// have finished scanning the network or blockchain.
func New(cs hdbConsensusSet, wallet modules.Wallet, tpool hdbTransactionPool, persistDir string) (*HostDB, error) {
	if cs == nil {
		return nil, errNilCS
	}
	if wallet == nil {
		return nil, errNilWallet
	}
	if tpool == nil {
		return nil, errNilTpool
	}

	hdb, err := newHostDB(&hdbWalletShim{w: wallet}, tpool, stdDialer{}, stdSleeper{}, newPersist(persistDir))
	if err != nil {
		return nil, err
	}
	err = hdb.initPersist(persistDir)
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

// newHostDB creates a HostDB using the provided dependencies. It does not
// have any side effects (i.e. it does not spawn background threads, perform
// I/O, or call stateful methods of its dependencies.)
func newHostDB(w hdbWallet, tpool hdbTransactionPool, d hdbDialer, s hdbSleeper, p hdbPersister) (*HostDB, error) {
	hdb := &HostDB{
		wallet:  w,
		tpool:   tpool,
		dialer:  d,
		sleeper: s,
		persist: p,

		contracts:   make(map[types.FileContractID]hostContract),
		activeHosts: make(map[modules.NetAddress]*hostNode),
		allHosts:    make(map[modules.NetAddress]*hostEntry),
		scanPool:    make(chan *hostEntry, scanPoolSize),
	}

	return hdb, nil
}
