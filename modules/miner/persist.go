package miner

import (
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
	// persist contains all of the persistent miner data.
	persistence struct {
		RecentChange  modules.ConsensusChangeID
		Height        types.BlockHeight
		Target        types.Target
		Address       types.UnlockHash
		BlocksFound   []types.BlockID
		UnsolvedBlock types.Block
	}
)

// initSettings loads the settings file if it exists and creates it if it
// doesn't.
func (m *Miner) initSettings() error {
	filename := filepath.Join(m.persistDir, settingsFile)
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return m.save()
	} else if err != nil {
		return err
	}
	return m.load()
}

// initPersist initializes the persistence of the miner.
func (m *Miner) initPersist() error {
	// Create the miner directory.
	err := os.MkdirAll(m.persistDir, 0700)
	if err != nil {
		return err
	}

	// Add a logger.
	m.log, err = persist.NewLogger(filepath.Join(m.persistDir, logFile))
	if err != nil {
		return err
	}

	return m.initSettings()
}

// load loads the miner persistence from disk.
func (m *Miner) load() error {
	return persist.LoadFile(settingsMetadata, &m.persist, filepath.Join(m.persistDir, settingsFile))
}

// save saves the miner persistence to disk.
func (m *Miner) save() error {
	return persist.SaveFile(settingsMetadata, m.persist, filepath.Join(m.persistDir, settingsFile))
}
