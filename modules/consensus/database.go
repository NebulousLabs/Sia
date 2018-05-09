package consensus

// database.go contains functions to initialize the database and report
// inconsistencies. All of the database-specific logic belongs here.

import (
	"errors"
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/persist"
)

var (
	errDBInconsistent = errors.New("database guard indicates inconsistency within database")
	errNilBucket      = errors.New("using a bucket that does not exist")
	errNilItem        = errors.New("requested item does not exist")
	errNonEmptyBucket = errors.New("cannot remove a map with objects still in it")
	errRepeatInsert   = errors.New("attempting to add an already existing item to the consensus set")
)

// replaceDatabase backs up the existing database and creates a new one.
func (cs *ConsensusSet) replaceDatabase(filename string) error {
	// Rename the existing database and create a new one.
	fmt.Println("Outdated consensus database... backing up and replacing")
	err := os.Rename(filename, filename+".bck")
	if err != nil {
		return errors.New("error while backing up consensus database: " + err.Error())
	}

	// Try again to create a new database, this time without checking for an
	// outdated database error.
	cs.db, err = database.Open(filename)
	if err != nil {
		return errors.New("error opening consensus database: " + err.Error())
	}
	return nil
}

// openDB loads the set database and populates it with the necessary buckets
func (cs *ConsensusSet) openDB(filename string) (err error) {
	cs.db, err = database.Open(filename)
	if err == persist.ErrBadVersion {
		return cs.replaceDatabase(filename)
	}
	if err != nil {
		return errors.New("error opening consensus database: " + err.Error())
	}
	return nil
}

// initDB is run if there is no existing consensus database, creating a
// database with all the required buckets and sane initial values.
func (cs *ConsensusSet) initDB(tx database.Tx) error {
	// If the database has already been initialized, there is nothing to do.
	// Initialization can be detected by looking for the genesis entry in the
	// change log.
	if _, exists := tx.ChangeEntry(cs.genesisEntry().ID()); exists {
		return nil
	}

	// Create the compononents of the database.
	err := cs.createConsensusDB(tx)
	if err != nil {
		return err
	}
	err = cs.createChangeLog(tx)
	if err != nil {
		return err
	}
	return nil
}
