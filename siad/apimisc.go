package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock           consensus.BlockID
	Height                 consensus.BlockHeight
	Target                 consensus.Target
	Depth                  consensus.Target
	EarliestLegalTimestamp consensus.Timestamp
}

// StateInfo returns a bunch of useful information about the state, doing
// read-only accesses. StateInfo does not lock the state mutex, which means
// that the data could potentially be weird on account of race conditions.
// Because it's just a read-only call, it will not adversely affect the state.
// If accurate data is paramount, SafeStateInfo() should be called, though this
// can adversely affect performance.
func (c *Core) StateInfo() StateInfo {
	return StateInfo{
		CurrentBlock: c.state.CurrentBlock().ID(),
		Height:       c.state.Height(),
		Target:       c.state.CurrentTarget(),
		Depth:        c.state.Depth(),
		EarliestLegalTimestamp: c.state.EarliestTimestamp(),
	}
}

func (d *daemon) updateCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := checkForUpdate()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, struct {
		Available bool
		Version   string
	}{available, version})
}

func (d *daemon) updateApplyHandler(w http.ResponseWriter, req *http.Request) {
	err := applyUpdate(req.FormValue("version"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeSuccess(w)
}

func (d *daemon) statusHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, d.core.StateInfo())
}

func (d *daemon) stopHandler(w http.ResponseWriter, req *http.Request) {
	writeSuccess(w)

	// send stop signal
	d.apiServer.Stop(1e9)
}

func (d *daemon) syncHandler(w http.ResponseWriter, req *http.Request) {
	// TODO: don't spawn multiple CatchUps
	if len(d.core.AddressBook()) == 0 {
		http.Error(w, "No peers available for syncing", 500)
		return
	}

	go d.core.CatchUp(d.core.RandomPeer())

	writeSuccess(w)
}
