package miningpool

import (
	"log"
	"os"
	"path/filepath"
)

// initPersist() initializes the persistence of the mining pool.
func (mp *MiningPool) initPersist() error {
	// Create the mining pool dir.
	err := os.MkdirAll(mp.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(mp.persistDir, "miner.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	mp.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	mp.log.Println("INFO: MiningPool logger opened, logging has started.")
	return nil
}
