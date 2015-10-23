package explorer

import (
	"os"
	"path/filepath"
)

// initPersist initializes the persistent structures of the explorer module.
func (e *Explorer) initPersist() error {
	// Make the persist directory
	err := os.MkdirAll(e.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initilize the database
	db, err := openDB(filepath.Join(e.persistDir, "blocks.db"))
	if err != nil {
		return err
	}
	e.db = db

	return nil
}
