package host

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

var persistMetadata = persist.Metadata{
	Header:  "Sia Host",
	Version: "0.3.3",
}

type savedHost struct {
	FileCounter  int
	Obligations  []contractObligation
	HostSettings modules.HostSettings
}

func (h *Host) save() error {
	sHost := savedHost{
		FileCounter:  h.fileCounter,
		Obligations:  make([]contractObligation, 0, len(h.obligationsByID)),
		HostSettings: h.HostSettings,
	}
	for _, obligation := range h.obligationsByID {
		sHost.Obligations = append(sHost.Obligations, obligation)
	}

	return persist.SaveFile(persistMetadata, sHost, filepath.Join(h.saveDir, "settings.json"))
}

func (h *Host) load() error {
	var sHost savedHost
	err := persist.LoadFile(persistMetadata, &sHost, filepath.Join(h.saveDir, "settings.json"))
	if err != nil {
		return err
	}

	h.spaceRemaining = sHost.HostSettings.TotalStorage
	h.fileCounter = sHost.FileCounter
	h.HostSettings = sHost.HostSettings
	// recreate maps
	for _, obligation := range sHost.Obligations {
		height := obligation.FileContract.WindowStart + StorageProofReorgDepth
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.ID] = obligation
		// update spaceRemaining
		h.spaceRemaining -= int64(obligation.FileContract.FileSize)
	}

	return nil
}
