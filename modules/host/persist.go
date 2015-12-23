package host

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// logFile establishes the name of the file that gets used for logging.
	logFile = modules.HostDir + ".log"
)

// Variables indicating the metadata header and the version of the data that
// has been saved to disk.
var persistMetadata = persist.Metadata{
	Header:  "Sia Host",
	Version: "0.4",
}

// persistence is the data that is kept when the host is restarted.
type persistence struct {
	BlockHeight    types.BlockHeight
	FileCounter    int64
	HostSettings   modules.HostSettings
	Profit         types.Currency
	SpaceRemaining int64
	Obligations    []contractObligation
	SecretKey      crypto.SecretKey
	PublicKey      types.SiaPublicKey
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	sHost := persistence{
		SpaceRemaining: h.spaceRemaining,
		FileCounter:    h.fileCounter,
		Profit:         h.profit,
		HostSettings:   h.settings,
		Obligations:    make([]contractObligation, 0, len(h.obligationsByID)),
		SecretKey:      h.secretKey,
		PublicKey:      h.publicKey,
	}
	for _, ob := range h.obligationsByID {
		// to avoid race conditions involving the obligation's mutex, copy it
		// manually into a new object
		obcopy := contractObligation{ID: ob.ID, FileContract: ob.FileContract, LastRevisionTxn: ob.LastRevisionTxn, Path: ob.Path}
		sHost.Obligations = append(sHost.Obligations, obcopy)
	}

	return persist.SaveFile(persistMetadata, sHost, filepath.Join(h.persistDir, "settings.json"))
}

// load extrats the save data from disk and populates the host.
func (h *Host) load() error {
	var sHost persistence
	err := persist.LoadFile(persistMetadata, &sHost, filepath.Join(h.persistDir, "settings.json"))
	if err != nil {
		return err
	}

	h.spaceRemaining = sHost.HostSettings.TotalStorage
	h.fileCounter = sHost.FileCounter
	h.settings = sHost.HostSettings
	h.profit = sHost.Profit
	// recreate maps
	for i := range sHost.Obligations {
		obligation := &sHost.Obligations[i] // both maps should use same pointer
		height := obligation.FileContract.WindowStart + StorageProofReorgDepth
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.ID] = obligation

		// update spaceRemaining
		if len(obligation.LastRevisionTxn.FileContractRevisions) > 0 { // COMPATv0.4.8
			h.spaceRemaining -= int64(obligation.LastRevisionTxn.FileContractRevisions[0].NewFileSize)
		}
	}
	h.secretKey = sHost.SecretKey
	h.publicKey = sHost.PublicKey

	return nil
}

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostSettings{
		TotalStorage: 10e9,     // 10 GB
		MaxFilesize:  100e9,    // 100 GB - deprecated field
		MaxDuration:  144 * 60, // 60 days
		WindowSize:   288,      // 48 hours
		Price:        defaultPrice,
		Collateral:   types.NewCurrency64(0),
	}
	h.spaceRemaining = h.settings.TotalStorage

	// Generate signing key, for revising contracts.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return err
	}
	h.secretKey = sk
	h.publicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// Save the defaults to disk.
	err = h.save()
	if err != nil {
		return err
	}
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
	h.log, err = persist.NewLogger(filepath.Join(h.persistDir, logFile))
	if err != nil {
		return err
	}

	// Load the prior persistance structures.
	err = h.load()
	if os.IsNotExist(err) {
		err = h.establishDefaults()
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}
