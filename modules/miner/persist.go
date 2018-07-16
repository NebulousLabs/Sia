package miner

import (
	"os"
	"path/filepath"
	"time"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/persist"
	"gitlab.com/NebulousLabs/Sia/types"
)

const (
	logFile        = modules.MinerDir + ".log"
	saveLoopPeriod = time.Minute * 2
	settingsFile   = modules.MinerDir + ".json"
)

var (
	settingsMetadata = persist.Metadata{
		Header:  "Miner Settings",
		Version: "0.5.0",
	}
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
		return m.saveSync()
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
	m.log, err = persist.NewFileLogger(filepath.Join(m.persistDir, logFile))
	if err != nil {
		return err
	}

	return m.initSettings()
}

// load loads the miner persistence from disk.
func (m *Miner) load() error {
	return persist.LoadJSON(settingsMetadata, &m.persist, filepath.Join(m.persistDir, settingsFile))
}

// saveSync saves the miner persistence to disk, and then syncs to disk.
func (m *Miner) saveSync() error {
	return persist.SaveJSON(settingsMetadata, m.persist, filepath.Join(m.persistDir, settingsFile))
}

// threadedSaveLoop periodically saves the miner persist.
func (m *Miner) threadedSaveLoop() {
	for {
		select {
		case <-m.tg.StopChan():
			return
		case <-time.After(saveLoopPeriod):
		}

		func() {
			err := m.tg.Add()
			if err != nil {
				return
			}
			defer m.tg.Done()

			m.mu.Lock()
			err = m.saveSync()
			m.mu.Unlock()
			if err != nil {
				m.log.Println("ERROR: Unable to save miner persist:", err)
			}
		}()
	}
}
