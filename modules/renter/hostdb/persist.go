package hostdb

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

var (
	// persistFilename defines the name of the file that holds the hostdb's
	// persistence.
	persistFilename = "hostdb.json"

	// persistMetadata defines the metadata that tags along with the most recent
	// version of the hostdb persistence file.
	persistMetadata = persist.Metadata{
		Header:  "HostDB Persistence",
		Version: "0.5",
	}
)

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	AllHosts   []modules.HostDBEntry
	LastChange modules.ConsensusChangeID
}

// persistData returns the data in the hostdb that will be saved to disk.
func (hdb *HostDB) persistData() (data hdbPersist) {
	data.AllHosts = hdb.hostTree.All()
	data.LastChange = hdb.lastChange
	return data
}

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save() error {
	return hdb.deps.saveFile(persistMetadata, hdb.persistData(), filepath.Join(hdb.persistDir, persistFilename))
}

// saveSync saves the hostdb persistence data to disk and then syncs to disk.
func (hdb *HostDB) saveSync() error {
	return hdb.deps.saveFileSync(persistMetadata, hdb.persistData(), filepath.Join(hdb.persistDir, persistFilename))
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	// Fetch the data from the file.
	var data hdbPersist
	err := hdb.deps.loadFile(persistMetadata, &data, filepath.Join(hdb.persistDir, persistFilename))
	if err != nil {
		return err
	}

	// Load each of the hosts into the host tree.
	for _, host := range data.AllHosts {
		err := hdb.hostTree.Insert(host)
		if err != nil {
			hdb.log.Debugln("ERROR: could not insert host while loading:", host.NetAddress)
		}
	}
	hdb.lastChange = data.LastChange
	return nil
}
