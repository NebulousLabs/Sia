package hostdb

import (
	"github.com/NebulousLabs/Sia/modules"
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
	return hdb.persist.save(hdb.persistData())
}

// saveSync saves the hostdb persistence data to disk and then syncs to disk.
func (hdb *HostDB) saveSync() error {
	return hdb.persist.saveSync(hdb.persistData())
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	var data hdbPersist
	err := hdb.persist.load(&data)
	if err != nil {
		return err
	}
	for _, host := range data.AllHosts {
		err := hdb.hostTree.Insert(host)
		if err != nil {
			hdb.log.Debugln("ERROR: could not insert host while loading:", host.NetAddress)
		}
	}
	hdb.lastChange = data.LastChange
	return nil
}
