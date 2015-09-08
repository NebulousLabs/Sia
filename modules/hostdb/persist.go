package hostdb

import (
	"log"
	"os"
	"path/filepath"
)

// initPersist initializes the persistence folder of the hostdb, including the
// logger.
func (hdb *HostDB) initPersist() error {
	// Create the hostdb dir if it does not exist.
	err := os.MkdirAll(hdb.persistDir, 0700)
	if err != nil {
		return err
	}

	// Start the logger, appending to the existing log file.
	logFile, err := os.OpenFile(filepath.Join(hdb.persistDir, "hostdb.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	hdb.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	hdb.log.Println("STARTUP: Hostdb logging has started.")
	return nil
}
