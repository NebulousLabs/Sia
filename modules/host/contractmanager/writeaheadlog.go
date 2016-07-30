package contractmanager

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
)

var (
	// errBadWalTxPw complains if an operation is executed on a walTx using an
	// incorrect password. This almost always means that at some point earlier
	// 'Close' was called on the Tx.
	errBadWalTxPw = errors.New("walTx operation attempted using a Tx with an outdated password")
)

type (
	writeAheadLog struct {
		// Actions that can be performed inside of a tx.
		//
		// Application of the changes can happen multiple times if there are
		// events like power outages or disk failures. Therefore, preserving
		// ACID properties requires that all changes defined in the WAL are
		// idempotent.
		slcs []sectorLocationChanges

		// Variables to coordinate ACID transcations. Multiple transactions may
		// be batched together, and none of them will return until the WAL has
		// atomically synced to disk. Syncing should only happen once, and a tx
		// should be waiting at most 3 seconds for a sync to begin.
		//
		// More here.
		firstTx   time.Time
		syncAct   sync.Once
		syncChan  chan struct{}
		syncCheck sync.Once

		// Utilities. The WAL needs access to the ContractManager because all
		// mutations to ACID fields of the contract manager happen through the
		// WAL.
		//
		// It's also importatnt that only one transaction be open on the WAL at
		// a time. A mutex helps to protect that, and then a password is used
		// to provide additional safety, to make sure that only one transaction
		// is being used and that old transactions are not still modifying the
		// WAL. The password is a sanity check more than anything else.
		cm *ContractManager
		mu sync.Mutex
		pw string
	}

	walTx struct {
		pw  string
		wal writeAheadLog
	}
)

func (wal *writeAheadLog) sync() {
	// so, this might be called on a timer. sync waits for someone to close the
	// sync prep channel? How does locking work then? So, sync prep... oh, just
	// use a sync.Once.

	// So, sync.Once => sync. Will be a no-op if all the 3 second timers try to
	// do it again, so long as they are using a pointer to the sync channel,
	// locking is safely happening, and the sync.Once is being replaced
	// correctly.
}

// Open will open the writeAheadLog for modifications. All modifications
// submitted between the call to Open and Close will be applied as a single
// ACID transaction to the contract manager.
func (wal *writeAheadLog) StartTransaction() *walTx {
	wal.mu.Lock()

	randBytes, err := crypto.RandBytes(12)
	if err != nil {
		cm.log.Critical("contract manager has no entropy")
	}
	wal.pw = string(randBytes)
	return &walTx{
		wal: wal,
		pw: wal.pw,
	}
}

// Close will finalize a writeAheadLog transaction, committing all of the
// changes to disk, ensuring that changes are ACID.
func (tx *walTx) Close() error {
	// Sanity check - the password of the tx should match the password of the
	// wal.
	if tx.pw != walTx {
		tx.wal.cm.log.Critical(errBadWalTxPw)
		return errBadWalTxPw
	}

	// Clear the password of the tx so that it cannot be used anymore to
	// perform operations on the WAL.
	tx.pw = "closed"

	// Grab the sync chan before notifying the wal that a tx has completed. The
	// nofitication may trigger a sync, which will replace the syncChan. We
	// want to know when the current changes are synced, and therefore we need
	// to grab the syncChan before it may get replaced.
	sc := tx.wal.syncChan
	tx.wal.notify()

	// Update is complete, the WAL can be unlocked to make room for another Tx.
	tx.wal.mu.Unlock()
	<-sc
}
