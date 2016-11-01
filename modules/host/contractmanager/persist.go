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
	// savedSettings contains fields that are saved atomically to disk inside
	// of the contract manager directory, alongside the WAL and log.
	savedSettings struct {
		SectorSalt     crypto.Hash
		StorageFolders []storageFolder
	}
)

// initSettings will set the default settings for the contract manager.
// initSettings should only be run for brand new contract maangers.
func (cm *ContractManager) initSettings() error {
	// Initialize the sector salt to a random value.
	crypto.Read(cm.sectorSalt[:])

	// Ensure that the initialized defaults have stuck by doing a SaveFileSync
	// with the new settings values.
	ss := cm.savedSettings()
	return build.ExtendErr("error saving contract manager after initialization", persist.SaveFileSync(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile)))
}

// loadSettings will load the contract manager settings.
func (cm *ContractManager) loadSettings() error {
	var ss savedSettings
	err := cm.dependencies.loadFile(settingsMetadata, &ss, filepath.Join(cm.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no settings file, this must be the first time that the
		// contract manager has been run. Initialize with default settings.
		return cm.initSettings()
	} else if err != nil {
		return build.ExtendErr("error loading the contract manager settings file", err)
	}

	// Copy the saved settings into the contract manager.
	cm.sectorSalt = ss.SectorSalt
	for _, sf := range ss.StorageFolders {
		cm.storageFolders[sf.Index] = &sf
		sf.file, err = os.OpenFile(filepath.Join(sf.Path, sectorFile), os.O_RDWR, 0700)
		if err != nil {
			return build.ExtendErr("error loading storage folder file handle", err)
		}
	}
	return nil
}

// loadSectorLocations will read the metadata portion of each storage folder
// file and load the sector location information into memory. Because the
// sectorLocations data is not saved to disk atomically, this operation must
// happen after the WAL has been loaded, as the WAL will repair any corruption
// and restore any missing data.
func (cm *ContractManager) loadSectorLocations() {
	// Each storage folder houses separate sector location data.
	for _, sf := range cm.storageFolders {
		// The storage folder's file should already be open from where the WAL
		// was doing repairs. For the sake of speed, read the whole sector
		// lookup table into memory.
		sectorLookupBytes := make([]byte, len(sf.Usage)*storageFolderGranularity*sectorMetadataDiskSize)
		// Seek to the beginning of the file and read the whole lookup table.
		// In the event of an error, continue directly to the next storage
		// folder.
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

		// The data is organized into sets of 64 sectors. The storage folder
		// Usage field is a bitfield that contains flipped bits in every index
		// that corresponds to an existing sector. If the sector does not
		// exist, the data is still represented in the sector lookup bytes,
		// it's just garbage data. The data is not guaranteed to be zerored
		// out.
		//
		// The outer loop iterates through each set of sectors. Each set of
		// sectors is represeted by one element in the usage array.
		readHead := 0
		for i, usage := range sf.Usage {
			// The inner loop iterates through every sector in a set of
			// sectors. Each sector is represent by one bit in the usage
			// element.
			usageMask := uint64(1)
			for j := 0; j < 64; j++ {
				// If the corresponding bit in the usage element is flipped,
				// there is a real sector here. Otherwise, the data can be
				// assumed to be garbage.
				if usage&usageMask == usageMask {
					// There is valid sector metadata here. The next 14 bytes
					// contain all information needed to piece together the
					// full sector location information.
					var id sectorID
					copy(id[:], sectorLookupBytes[readHead:readHead+12])
					count := binary.LittleEndian.Uint16(sectorLookupBytes[readHead+12 : readHead+14])
					sl := sectorLocation{
						index:         uint32(i*64 + j),
						storageFolder: sf.Index,
						count:         count,
					}
					// Add the sector to the sector location map.
					cm.sectorLocations[id] = sl
					sf.sectors += 1
				}

				// Advance the read head, and then check the next bit of the
				// usage element.
				readHead += sectorMetadataDiskSize
				usageMask = usageMask << 1
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
