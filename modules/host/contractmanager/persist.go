package contractmanager

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync/atomic"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/persist"
	"gitlab.com/NebulousLabs/fastrand"
)

type (
	// savedStorageFolder contains fields that are saved automatically to disk
	// for each storage folder.
	savedStorageFolder struct {
		Index uint16
		Path  string
		Usage []uint64
	}

	// savedSettings contains fields that are saved atomically to disk inside
	// of the contract manager directory, alongside the WAL and log.
	savedSettings struct {
		SectorSalt     crypto.Hash
		StorageFolders []savedStorageFolder
	}
)

// savedStorageFolder returns the persistent version of the storage folder.
func (sf *storageFolder) savedStorageFolder() savedStorageFolder {
	ssf := savedStorageFolder{
		Index: sf.index,
		Path:  sf.path,
		Usage: make([]uint64, len(sf.usage)),
	}
	copy(ssf.Usage, sf.usage)
	return ssf
}

// initSettings will set the default settings for the contract manager.
// initSettings should only be run for brand new contract maangers.
func (cm *ContractManager) initSettings() error {
	// Initialize the sector salt to a random value.
	fastrand.Read(cm.sectorSalt[:])

	// Ensure that the initialized defaults have stuck.
	ss := cm.savedSettings()
	err := persist.SaveJSON(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
	if err != nil {
		cm.log.Println("ERROR: unable to initialize settings file for contract manager:", err)
		return build.ExtendErr("error saving contract manager after initialization", err)
	}
	return nil
}

// loadSettings will load the contract manager settings.
func (cm *ContractManager) loadSettings() error {
	var ss savedSettings
	err := cm.dependencies.LoadFile(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no settings file, this must be the first time that the
		// contract manager has been run. Initialize with default settings.
		return cm.initSettings()
	} else if err != nil {
		cm.log.Println("ERROR: unable to load the contract manager settings file:", err)
		return build.ExtendErr("error loading the contract manager settings file", err)
	}

	// Copy the saved settings into the contract manager.
	cm.sectorSalt = ss.SectorSalt
	for i := range ss.StorageFolders {
		sf := new(storageFolder)
		sf.index = ss.StorageFolders[i].Index
		sf.path = ss.StorageFolders[i].Path
		sf.usage = ss.StorageFolders[i].Usage
		sf.metadataFile, err = cm.dependencies.OpenFile(filepath.Join(ss.StorageFolders[i].Path, metadataFile), os.O_RDWR, 0700)
		if err != nil {
			// Mark the folder as unavailable and log an error.
			atomic.StoreUint64(&sf.atomicUnavailable, 1)
			cm.log.Printf("ERROR: unable to open the %v sector metadata file: %v\n", sf.path, err)
		}
		sf.sectorFile, err = cm.dependencies.OpenFile(filepath.Join(ss.StorageFolders[i].Path, sectorFile), os.O_RDWR, 0700)
		if err != nil {
			// Mark the folder as unavailable and log an error.
			atomic.StoreUint64(&sf.atomicUnavailable, 1)
			cm.log.Printf("ERROR: unable to open the %v sector file: %v\n", sf.path, err)
			if sf.metadataFile != nil {
				sf.metadataFile.Close()
			}
		}
		sf.availableSectors = make(map[sectorID]uint32)
		cm.storageFolders[sf.index] = sf
	}
	return nil
}

// loadSectorLocations will read the metadata portion of each storage folder
// file and load the sector location information into memory.
func (cm *ContractManager) loadSectorLocations(sf *storageFolder) {
	// Read the sector lookup table for this storage folder into memory.
	sectorLookupBytes, err := readFullMetadata(sf.metadataFile, len(sf.usage)*storageFolderGranularity)
	if err != nil {
		atomic.AddUint64(&sf.atomicFailedReads, 1)
		atomic.StoreUint64(&sf.atomicUnavailable, 1)
		err = build.ComposeErrors(err, sf.metadataFile.Close())
		err = build.ComposeErrors(err, sf.sectorFile.Close())
		cm.log.Printf("ERROR: unable to read sector metadata for folder %v: %v\n", sf.path, err)
		return
	}
	atomic.AddUint64(&sf.atomicSuccessfulReads, 1)

	// Iterate through the sectors that are in-use and read their storage
	// locations into memory.
	sf.sectors = 0 // may be non-zero from WAL operations - they will be double counted here if not reset.
	for _, sectorIndex := range usageSectors(sf.usage) {
		readHead := sectorMetadataDiskSize * sectorIndex
		var id sectorID
		copy(id[:], sectorLookupBytes[readHead:readHead+12])
		count := binary.LittleEndian.Uint16(sectorLookupBytes[readHead+12 : readHead+14])
		sl := sectorLocation{
			index:         sectorIndex,
			storageFolder: sf.index,
			count:         count,
		}

		// Add the sector to the sector location map.
		cm.sectorLocations[id] = sl
		sf.sectors++
	}
	atomic.StoreUint64(&sf.atomicUnavailable, 0)
}

// savedSettings returns the settings of the contract manager in an
// easily-serializable form.
func (cm *ContractManager) savedSettings() savedSettings {
	ss := savedSettings{
		SectorSalt: cm.sectorSalt,
	}
	for _, sf := range cm.storageFolders {
		// Unset all of the usage bits in the storage folder for the queued sectors.
		for _, sectorIndex := range sf.availableSectors {
			sf.clearUsage(sectorIndex)
		}

		// Copy over the storage folder.
		ss.StorageFolders = append(ss.StorageFolders, sf.savedStorageFolder())

		// Re-set all of the usage bits for the queued sectors.
		for _, sectorIndex := range sf.availableSectors {
			sf.setUsage(sectorIndex)
		}
	}
	return ss
}
