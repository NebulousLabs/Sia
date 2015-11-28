package hostdb

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/persist"
)

const persistFilename = "hostdb.json"

var saveMetadata = persist.Metadata{
	Header:  "HostDB Persistence",
	Version: "0.5",
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	var data struct {
		Contracts []hostContract
	}
	for _, hc := range hdb.contracts {
		// to avoid race conditions involving the contract's mutex, copy it
		// manually into a new object
		data.Contracts = append(data.Contracts, hostContract{
			ID:              hc.ID,
			FileContract:    hc.FileContract,
			LastRevisionTxn: hc.LastRevisionTxn,
			SecretKey:       hc.SecretKey,
		})
	}
	return persist.SaveFile(saveMetadata, data, filepath.Join(hdb.persistDir, persistFilename))
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	var data struct {
		Contracts []hostContract
	}
	err := persist.LoadFile(saveMetadata, &data, filepath.Join(hdb.persistDir, persistFilename))
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
func (hdb *HostDB) initPersist() error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(hdb.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(hdb.persistDir, "hostdb.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
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
