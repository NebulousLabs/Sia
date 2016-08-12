package contractmanager

import (
	"encoding/binary"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
)

type (
	// savedSettings contains all of the contract manager persistent details
	// that are single fields or otherwise do not need to go into a database or
	// onto another disk.
	savedSettings struct {
		SectorSalt     crypto.Hash
		StorageFolders []storageFolder
	}
)

// initSettings will set the default settings for the contract manager.
// initSettings should only be run for brand new contract maangers.
func (cm *ContractManager) initSettings() error {
	// Typically any time a change is made to the persistent state of the
	// contract manager, the write ahead log should be used. We have a unique
	// situation where there is a brand new contract manager, and we can rely
	// on the safety features of persist.SaveFileSync to be certain that
	// everything will be saved cleanly to disk or not at all before the
	// function is returned, therefore the WAL is not used, and this saves some
	// code, especially regarding changes to the sector salt. Aside from
	// initialization, the sector salt is never changed.

	// Initialize the sector salt to a random value.
	_, err := cm.dependencies.randRead(cm.sectorSalt[:])
	if err != nil {
		return build.ExtendErr("error creating salt for contract manager", err)
	}

	// Ensure that the initialized defaults have stuck by doing a SaveFileSync
	// with the new settings values.
	ss := cm.savedSettings()
	return build.ExtendErr("error saving contract manager after initialization", persist.SaveFileSync(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile)))
}

// loadAtomicPersistence will load all data that has been saved to disk
// atomically. This usually means that the data was saved with a copy-on-write
// call.
//
// The WAL is not included in atomic persistence, though it is saved
// atomically. The WAL is a recovery mechanism that gets used after all other
// state has been loaded, and restores consistency to the host.
func (cm *ContractManager) loadAtomicPersistence() error {
	var ss savedSettings
	err := cm.dependencies.loadFile(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no settings file, this must be the first time that the
		// contract manager has been run. Initialize the contracter with
		// default settings.
		return cm.initSettings()
	} else if err != nil {
		return build.ExtendErr("error loading the contract manager settings file", err)
	}

	// Copy the saved settings into the contract manager. Any saved settings
	// that are in the file on disk have also been committed successfully to
	// the other state of the contract manager.
	cm.sectorSalt = ss.SectorSalt
	for _, sf := range ss.StorageFolders {
		cm.storageFolders[sf.Index] = &sf
		sf.file, err = os.OpenFile(filepath.Join(sf.Path, sectorFile), os.O_RDWR, 0700)
		if err != nil {
			return build.ExtendErr("error loading storage folder file handle", err)
		}
	}

	// If extending the contract manager, any in-memory changes should be
	// loaded here, before wal.load() is called. The first thing that
	// wal.load() will do is check for a previous WAL, which indicates an
	// unclean shutdown. wal.load() depends on all in-memory resources being
	// fully loaded already.

	return nil
}

// loadSectorLocations will read the metadata portion of each storage folder
// file and load the sector location information into memory. Note that this
// function should only be called after the WAL has restored consistency to the
// state of the storage folder database.
func (cm *ContractManager) loadSectorLocations() {
	// Each storage folder houses separate sector location data.
	for _, sf := range cm.storageFolders {
		// The storage folder's file should already be open from where the WAL
		// was doing repairs. For the sake of speed, read the whole sector
		// lookup table into memory.
		sectorLookupBytes := make([]byte, len(sf.Usage)*storageFolderGranularity*sectorMetadataDiskSize)
		// Seek to the beginning of the file and read the whole lookup table.
		// In the event of a failure, assume disk error and continue to the
		// next storage folder.
		_, err := sf.file.Seek(0, 0)
		if err != nil {
			cm.log.Println("Error: difficulty seeking in storge folder file during startup", err)
			sf.failedReads++
			continue
		}
		_, err = sf.file.Read(sectorLookupBytes)
		if err != nil {
			cm.log.Println("Error: difficulty reading from storge folder file during startup", err)
			sf.failedReads++
			continue
		}

		// Parse the data storageFolderGranularity at a time. Compare against
		// Usage to determine if a sector is represented by the metadata at a
		// given point, and if it is load the metadata into the sectorLocations
		// map.
		readHead := 0
		for i, usage := range sf.Usage {
			usageMask := uint64(1)
			for j := 0; j < 64; j++ {
				if usage&usageMask == usage {
					// There is valid sector metadata here.
					var id sectorID
					copy(id[:], sectorLookupBytes[readHead:readHead+12])
					count := binary.LittleEndian.Uint16(sectorLookupBytes[readHead+12 : readHead+14])
					sl := sectorLocation{
						index:         uint32(i*64 + j),
						storageFolder: sf.Index,
						count:         count,
					}
					cm.sectorLocations[id] = sl
				}
				usageMask = usageMask << 1
				readHead += sectorMetadataDiskSize
			}
		}
	}
}

// savedSettings returns the settings of the contract manager in an
// easily-serializable form.
func (cm *ContractManager) savedSettings() savedSettings {
	ss := savedSettings{
		SectorSalt: cm.sectorSalt,
	}
	for _, sf := range cm.storageFolders {
		ss.StorageFolders = append(ss.StorageFolders, *sf)
	}
	return ss
}
