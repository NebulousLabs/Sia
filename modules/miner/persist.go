package miner

import (
	"log"
	"os"
	"path/filepath"
)

// initPersist initializes the persistence of the miner.
func (m *Miner) initPersist() error {
	// Create the miner dir.
	err := os.MkdirAll(m.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(m.persistDir, "miner.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	m.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	m.log.Println("INFO: Miner logger opened, logging has started.")
	return nil
}
