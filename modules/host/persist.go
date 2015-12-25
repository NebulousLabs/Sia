package host

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
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
	Version: "0.5",
}

// persistence is the data that is kept when the host is restarted.
type persistence struct {
	// Host Context.
	BlockHeight  types.BlockHeight
	RecentChange modules.ConsensusChangeID
	PublicKey    types.SiaPublicKey
	SecretKey    crypto.SecretKey
	Settings     modules.HostSettings

	// File Management.
	FileCounter int64
	Obligations []contractObligation

	// Statistics.
	Profit types.Currency
}

// getObligations returns a slice containing all of the contract obligations
// currently being tracked by the host.
func (h *Host) getObligations() []contractObligation {
	cos := make([]contractObligation, 0, len(h.obligationsByID))
	for _, ob := range h.obligationsByID {
		cos = append(cos, contractObligation{
			ID:              ob.ID,
			FileContract:    ob.FileContract,
			LastRevisionTxn: ob.LastRevisionTxn,
			Path:            ob.Path,
		})
	}
	return cos
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	p := persistence{
		BlockHeight:  h.blockHeight,
		RecentChange: h.recentChange,
		PublicKey:    h.publicKey,
		SecretKey:    h.secretKey,
		Settings:     h.settings,

		FileCounter: h.fileCounter,
		Obligations: h.getObligations(),

		Profit: h.profit,
	}
	return persist.SaveFile(persistMetadata, p, filepath.Join(h.persistDir, "settings.json"))
}

// loadObligations loads file contract obligations from the persistent file
// into the host.
func (h *Host) loadObligations(cos []contractObligation) {
	// Clear the existing obligations maps.
	for i := range cos {
		obligation := &cos[i] // both maps should use same pointer
		height := obligation.FileContract.WindowStart + StorageProofReorgDepth
		// Sanity check - if the height is below the current height, then set
		// the height to current height + 3. This makes sure that all file
		// contracts will eventually be hit or garbage collected by the host,
		// even if a bug means that they aren't acted upon at the right moment.
		if build.DEBUG && height < h.blockHeight {
			panic("host settings file is inconsistent")
		} else if height < h.blockHeight {
			height = h.blockHeight + 3
		}
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.ID] = obligation

		// update spaceRemaining to account for the storage held by this
		// obligation.
		if len(obligation.LastRevisionTxn.FileContractRevisions) > 0 {
			h.spaceRemaining -= int64(obligation.LastRevisionTxn.FileContractRevisions[0].NewFileSize)
		}
	}
}

// load extrats the save data from disk and populates the host.
func (h *Host) load() error {
	p := new(persistence)
	err := persist.LoadFile(persistMetadata, p, filepath.Join(h.persistDir, "settings.json"))
	if err != nil {
		return err
	}

	// Copy over the host context.
	h.blockHeight = p.BlockHeight
	h.recentChange = p.RecentChange
	h.publicKey = p.PublicKey
	h.secretKey = p.SecretKey
	h.settings = p.Settings

	// Copy over the file management. The space remaining is recalculated from
	// disk instead of being saved, to maximize the potential usefulness of
	// restarting Sia as a means of eliminating unkonwn errors.
	h.fileCounter = p.FileCounter
	h.spaceRemaining = p.Settings.TotalStorage
	h.loadObligations(p.Obligations)

	// Copy over statistics.
	h.profit = p.Profit

	return nil
}

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostSettings{
		TotalStorage: defaultTotalStorage,
		MaxDuration:  defaultMaxDuration,
		WindowSize:   defaultWindowSize,
		Price:        defaultPrice,
		Collateral:   defaultCollateral,
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
