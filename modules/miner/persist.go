package miner

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	logFile      = modules.MinerDir + ".log"
	settingsFile = modules.MinerDir + ".json"
)

var (
	settingsMetadata = persist.Metadata{"Miner Settings", "0.5.0"}
)

type (
	// Persist contains all of the persistent miner data.
	Persist struct {
		RecentChange  modules.ConsensusChangeID
		Height        types.BlockHeight
		Target        types.Target
		Address       types.UnlockHash
		BlocksFound   []types.BlockID
		UnsolvedBlock types.Block
	}
)

// initPersist initializes the persistence of the miner.
func (m *Miner) initPersist() error {
	// Create the miner dir.
	err := os.MkdirAll(m.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(m.persistDir, logFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	m.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	m.log.Println("STARTUP: Miner logger opened, logging has started.")

	return m.save()
}

// load loads the miner persistence from disk.
func (m *Miner) load() error {
	return persist.LoadFile(settingsMetadata, &m.persist, filepath.Join(m.persistDir, settingsFile))
}

// save saves the miner persistence to disk.
func (m *Miner) save() error {
	return persist.SaveFile(settingsMetadata, m.persist, filepath.Join(m.persistDir, settingsFile))
}
