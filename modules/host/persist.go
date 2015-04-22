package host

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

type savedHost struct {
	SpaceRemaining int64
	FileCounter    int
	Obligations    []contractObligation
	HostSettings   modules.HostSettings
}

func (h *Host) save() (err error) {
	sHost := savedHost{
		SpaceRemaining: h.spaceRemaining,
		FileCounter:    h.fileCounter,
		Obligations:    make([]contractObligation, 0, len(h.obligationsByID)),
		HostSettings:   h.HostSettings,
	}
	for _, obligation := range h.obligationsByID {
		sHost.Obligations = append(sHost.Obligations, obligation)
	}

	return encoding.WriteFile(filepath.Join(h.saveDir, "settings.dat"), sHost)
}

func (h *Host) load() error {
	var sHost savedHost
	err := encoding.ReadFile(filepath.Join(h.saveDir, "settings.dat"), &sHost)
	if err != nil {
		return err
	}

	h.spaceRemaining = sHost.SpaceRemaining
	h.fileCounter = sHost.FileCounter
	h.HostSettings = sHost.HostSettings
	// recreate maps
	for _, obligation := range sHost.Obligations {
		height := obligation.FileContract.WindowStart + StorageProofReorgDepth
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.ID] = obligation
	}

	return nil
}
