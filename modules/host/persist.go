package host

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
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
	SecretKey      crypto.SecretKey
	PublicKey      types.SiaPublicKey
}

func (h *Host) save() error {
	sHost := savedHost{
		SpaceRemaining: h.spaceRemaining,
		FileCounter:    h.fileCounter,
		Profit:         h.profit,
		HostSettings:   h.HostSettings,
		Obligations:    make([]contractObligation, 0, len(h.obligationsByID)),
		SecretKey:      h.secretKey,
		PublicKey:      h.publicKey,
	}
	for _, obligation := range h.obligationsByID {
		sHost.Obligations = append(sHost.Obligations, obligation)
	}

	return persist.SaveFile(persistMetadata, sHost, filepath.Join(h.persistDir, "settings.json"))
}

func (h *Host) load() error {
	var sHost savedHost
	err := persist.LoadFile(persistMetadata, &sHost, filepath.Join(h.persistDir, "settings.json"))
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
	h.secretKey = sHost.SecretKey
	h.publicKey = sHost.PublicKey

	return nil
}

// initPersist handles all of the persistence initialization, such as creating
// the persistance directory and starting the logger.
func (h *Host) initPersist() error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(h.persistDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	logFile, err := os.OpenFile(filepath.Join(h.persistDir, "host.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return err
	}
	h.log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	h.log.Println("STARTUP: Host has started logging")

	// Load the prior persistance structures.
	err = h.load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
