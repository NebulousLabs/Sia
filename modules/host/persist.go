package host

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var persistMetadata = persist.Metadata{
	Header:  "Sia Host",
	Version: "0.4",
}

type savedHost struct {
	SpaceRemaining int64
	FileCounter    int
	Profit         types.Currency
	HostSettings   modules.HostSettings
	Obligations    []contractObligation
	MasterKey      types.SiaPublicKey
}

func (h *Host) save() error {
	sHost := savedHost{
		SpaceRemaining: h.spaceRemaining,
		FileCounter:    h.fileCounter,
		Profit:         h.profit,
		HostSettings:   h.HostSettings,
		Obligations:    make([]contractObligation, 0, len(h.obligationsByID)),
		MasterKey:      h.masterKey,
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
	h.profit = sHost.Profit
	// recreate maps
	for _, obligation := range sHost.Obligations {
		height := obligation.FileContract.WindowStart + StorageProofReorgDepth
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.ID] = obligation
		// update spaceRemaining
		h.spaceRemaining -= int64(obligation.FileContract.FileSize)
	}
	h.masterKey = sHost.MasterKey

	return nil
}
