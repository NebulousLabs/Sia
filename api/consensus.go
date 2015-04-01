package main

import (
	"net/http"

	"github.com/NebulousLabs/Sia/consensus"
)

// consensusStatusHandler handles the API call asking for the consensus status.
func (d *daemon) consensusStatusHandler(w http.ResponseWriter, req *http.Request) {
	currentBlock := d.state.CurrentBlock().ID()
	target := d.state.CurrentTarget()
	writeJSON(w, struct {
		Height       consensus.BlockHeight
		CurrentBlock consensus.BlockID
		Target       consensus.Target
	}{
		d.state.Height(),
		currentBlock,
		target,
	})
}
