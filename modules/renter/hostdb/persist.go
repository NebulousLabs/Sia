package hostdb

import (
	"log"
	"os"
	"path/filepath"
)

type hdbPersist struct {
	Contracts []hostContract
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	var data hdbPersist
	for _, hc := range hdb.contracts {
		data.Contracts = append(data.Contracts, hc)
	}
	return hdb.persist.save(data)
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	var data hdbPersist
	err := hdb.persist.load(&data)
	if err != nil {
		return err
	}
	for _, hc := range data.Contracts {
		hdb.contracts[hc.ID] = hc
	}
	return nil
}

// initPersist handles all of the persistence initialization, such as creating
// the persistance directory and starting the logger.
func (hdb *HostDB) initPersist(dir string) error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(dir, "hostdb.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	hdb.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	hdb.log.Println("STARTUP: HostDB has started logging")

	// Load the prior persistance structures.
	err = hdb.load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
