package hostdb

import (
	"github.com/NebulousLabs/Sia/modules"
)

// hdbPersist defines what HostDB data persists across sessions.
type hdbPersist struct {
	AllHosts    []hostEntry
	ActiveHosts []hostEntry
	LastChange  modules.ConsensusChangeID
}

// persistData returns the data in the hostdb that will be saved to disk.
func (hdb *HostDB) persistData() hdbPersist {
	var data hdbPersist
	for _, entry := range hdb.allHosts {
		data.AllHosts = append(data.AllHosts, *entry)
	}
	for _, entry := range hdb.activeHosts {
		data.ActiveHosts = append(data.ActiveHosts, *entry)
	}
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
	for i := range data.AllHosts {
		hdb.allHosts[data.AllHosts[i].NetAddress] = &data.AllHosts[i]
	}
	for i := range data.ActiveHosts {
		host := data.AllHosts[i]
		hdb.activeHosts[host.NetAddress] = &host
		hdb.hostTree.Insert(host.HostDBEntry)
	}
	hdb.lastChange = data.LastChange
	return nil
}
