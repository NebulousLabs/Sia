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

// save saves the hostdb persistence data to disk.
func (hdb *HostDB) save(fsync bool) error {
	var data hdbPersist
	for _, entry := range hdb.allHosts {
		data.AllHosts = append(data.AllHosts, *entry)
	}
	for _, node := range hdb.activeHosts {
		data.ActiveHosts = append(data.ActiveHosts, *node.hostEntry)
	}
	data.LastChange = hdb.lastChange
	return hdb.persist.save(data, fsync)
}

// load loads the hostdb persistence data from disk.
func (hdb *HostDB) load() error {
	var data hdbPersist
	err := hdb.persist.load(&data)
	if err != nil {
		return err
	}
	for _, entry := range data.AllHosts {
		hdb.allHosts[entry.NetAddress] = &entry
	}
	for _, entry := range data.ActiveHosts {
		hdb.insertNode(&entry)
	}
	hdb.lastChange = data.LastChange
	return nil
}
