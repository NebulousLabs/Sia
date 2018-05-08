package database

import (
	"github.com/NebulousLabs/Sia/persist"
	"github.com/coreos/bbolt"
)

// A DB is a database suitable for storing consensus data.
type DB interface {
	Update(func(Tx) error) error
	View(func(Tx) error) error
	Close() error
}

// Open opens a consensus database.
func Open(filename string) (DB, error) {
	pdb, err := persist.OpenDatabase(persist.Metadata{
		Header:  "Consensus Set Database",
		Version: "0.5.0",
	}, filename)
	return boltWrapper{pdb.DB}, err
}

// boltWrapper wraps bolt.DB to make it satisfy the DB interface.
type boltWrapper struct {
	db *bolt.DB
}

func (w boltWrapper) Update(fn func(Tx) error) error {
	return w.db.Update(func(tx *bolt.Tx) error {
		return fn(txWrapper{tx})
	})
}

func (w boltWrapper) View(fn func(Tx) error) error {
	return w.db.View(func(tx *bolt.Tx) error {
		return fn(txWrapper{tx})
	})
}

func (w boltWrapper) Close() error {
	return w.db.Close()
}
