package consensus

// database.go contains functions to initialize the database and report
// inconsistencies. All of the database-specific logic belongs here.

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/persist"

	"github.com/NebulousLabs/bolt"
)

var (
	errRepeatInsert   = errors.New("attempting to add an already existing item to the consensus set")
	errNilBucket      = errors.New("using a bucket that does not exist")
	errNilItem        = errors.New("requested item does not exist")
	errDBInconsistent = errors.New("database guard indicates inconsistency within database")
	errNonEmptyBucket = errors.New("cannot remove a map with objects still in it")

	dbMetadata = persist.Metadata{
		Version: "0.4.3",
		Header:  "Consensus Set Database",
	}
)

type (
	// dbBucket represents a collection of key/value pairs inside the database.
	dbBucket interface {
		Get(key []byte) []byte
	}

	// dbTx represents a read-only transaction on the database that can be used
	// for retrieving values.
	dbTx interface {
		Bucket(name []byte) dbBucket
	}

	// boltTxWrapper wraps a bolt.Tx so that it matches the dbTx interface. The
	// wrap is necessary because bolt.Tx.Bucket() returns a fixed type
	// (bolt.Bucket), but we want it to return an interface (dbBucket).
	boltTxWrapper struct {
		tx *bolt.Tx
	}
)

// Bucket returns the dbBucket associated with the given bucket name.
func (b boltTxWrapper) Bucket(name []byte) dbBucket {
	return b.tx.Bucket(name)
}

// openDB loads the set database and populates it with the necessary buckets
func (cs *ConsensusSet) openDB(filename string) (err error) {
	cs.db, err = persist.OpenDatabase(dbMetadata, filename)
	return err
}

// dbInitialized returns true if the database appears to be initialized, false
// if not. Checking for the existence of the siafund pool bucket is typically
// sufficient to determine whether the database has gone through the
// initialization process.
func dbInitialized(tx *bolt.Tx) bool {
	return tx.Bucket(SiafundPool) != nil
}

// initDatabase is run when the database. This has become the true
// init function for consensus set
func (cs *ConsensusSet) initDB(tx *bolt.Tx) error {
	// Create the compononents of the database.
	err := cs.createConsensusDB(tx)
	if err != nil {
		return err
	}
	err = createChangeLog(tx)
	if err != nil {
		return err
	}

	// Place a 'false' in the consistency bucket to indicate that no
	// inconsistencies have been found.
	err = tx.Bucket(Consistency).Put(Consistency, encoding.Marshal(false))
	if err != nil {
		return err
	}
	return nil
}

// inconsistencyDetected indicates whether inconsistency has been detected
// within the database.
func inconsistencyDetected(tx *bolt.Tx) (detected bool) {
	inconsistencyBytes := tx.Bucket(Consistency).Get(Consistency)
	err := encoding.Unmarshal(inconsistencyBytes, &detected)
	if build.DEBUG && err != nil {
		panic(err)
	}
	return detected
}

// markInconsistency flags the database to indicate that inconsistency has been
// detected.
func markInconsistency(tx *bolt.Tx) {
	// Place a 'true' in the consistency bucket to indicate that
	// inconsistencies have been found.
	err := tx.Bucket(Consistency).Put(Consistency, encoding.Marshal(true))
	if build.DEBUG && err != nil {
		panic(err)
	}

}
