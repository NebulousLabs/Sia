package contractmanager

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
)

type (
	// savedSettings contains the contract manager settings that get saved to
	// disk.
	savedSettings struct {
		SectorSalt      crypto.Hash
		StorageFolders  []*storageFolder
	}

	// sectorLocationEntry contains all the information necessary to add an
	// entry to the sectorLocations map in the contract manager.
	sectorLocationEntry struct {
		index         uint32
		sectorID      string
		storageFolder uint16
	}
)

// initSettings will set the default settings for the contract manager.
// initSettings should only be run for brand new contract maangers.
func (cm *ContractManager) initSettings() error {
	// Typically any time a change is made to the persistent state of the
	// contract manager, the write ahead log should be used. We have a unique
	// situation where there is a brand new contract manager, and we can rely
	// on the safety features of persist.SaveFile to be certain that everything
	// will be saved cleanly to disk or not at all before the function is
	// returned, therefore the write ahead log is not used.

	// Initialize the sector salt to a random value.
	_, err := cm.dependencies.randRead(cm.sectorSalt[:])
	if err != nil {
		return err
	}

	// Ensure that the initialized defaults have stuck by doing a SaveFileSync
	// with the new settings values.
	ss := savedSettings{
		SectorSalt:     cm.sectorSalt,
		StorageFolders: cm.storageFolders,
	}
	return persist.SaveFileSync(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
}

// load will pull all saved state from the contract manager off of the disk and
// into memory. The contract manager has a write ahead log, so after the saved
// state is loaded, the write ahead log will be checked and applied, to make
// sure that any actions have been fully atomic.
func (cm *ContractManager) load() error {
	// Load the most recent saved settings into the contract manager.
	var ss savedSettings
	err := cm.dependencies.loadFile(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no settings file, this must be the first time that the
		// contract manager has been run. Initialize the contracter with
		// default settings.
		return cm.initSettings()
	} else if err != nil {
		return err
	}

	// Copy the saved settings into the contract manager.
	cm.sectorSalt = ss.SectorSalt
	cm.storageFolders = ss.StorageFolders

	// TODO: Load the sector locations from the various storage folders.

	// TODO: Open up the WAL, check the checksum, and apply any outstanding
	// updates. Then delete the WAL. Remember that WAL updates need to be
	// idempotent.

	return nil
}
