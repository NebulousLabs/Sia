package host

import (
	"os"
	"path/filepath"
	"sync/atomic"

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
	// Consensus Tracking.
	BlockHeight  types.BlockHeight
	RecentChange modules.ConsensusChangeID

	// Host Identity.
	NetAddress modules.NetAddress
	PublicKey  types.SiaPublicKey
	SecretKey  crypto.SecretKey

	// File Management.
	Obligations []*contractObligation

	// Statistics.
	FileCounter int64
	LostRevenue types.Currency
	Revenue     types.Currency

	// RPC Tracking.
	ErroredCalls   uint64
	MalformedCalls uint64
	DownloadCalls  uint64
	RenewCalls     uint64
	ReviseCalls    uint64
	SettingsCalls  uint64
	UploadCalls    uint64

	// Utilities.
	Settings modules.HostSettings
}

// getObligations returns a slice containing all of the contract obligations
// currently being tracked by the host.
func (h *Host) getObligations() []*contractObligation {
	cos := make([]*contractObligation, 0, len(h.obligationsByID))
	for _, ob := range h.obligationsByID {
		cos = append(cos, ob)
	}
	return cos
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	p := persistence{
		// Consensus Tracking.
		BlockHeight:  h.blockHeight,
		RecentChange: h.recentChange,

		// Host Identity.
		NetAddress: h.netAddress,
		PublicKey:  h.publicKey,
		SecretKey:  h.secretKey,

		// File Management.
		Obligations: h.getObligations(),

		// Statistics.
		FileCounter: h.fileCounter,
		LostRevenue: h.lostRevenue,
		Revenue:     h.revenue,

		// RPC Tracking.
		ErroredCalls:   atomic.LoadUint64(&h.atomicErroredCalls),
		MalformedCalls: atomic.LoadUint64(&h.atomicMalformedCalls),
		DownloadCalls:  atomic.LoadUint64(&h.atomicDownloadCalls),
		RenewCalls:     atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:    atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:  atomic.LoadUint64(&h.atomicSettingsCalls),
		UploadCalls:    atomic.LoadUint64(&h.atomicUploadCalls),

		// Utilities.
		Settings: h.settings,
	}
	return persist.SaveFile(persistMetadata, p, filepath.Join(h.persistDir, "settings.json"))
}

// loadObligations loads file contract obligations from the persistent file
// into the host.
func (h *Host) loadObligations(cos []*contractObligation) {
	for i := range cos {
		// Store the obligation in the obligations list.
		obligation := cos[i] // all objects should reference the same obligation
		h.obligationsByID[obligation.ID] = obligation

		// Update spaceRemaining to account for the storage held by this
		// obligation.
		h.spaceRemaining -= int64(obligation.fileSize())

		// Update anticipated revenue to reflect the revenue in this file
		// contract.
		h.anticipatedRevenue = h.anticipatedRevenue.Add(obligation.value())
	}
}

// load extrats the save data from disk and populates the host.
func (h *Host) load() error {
	p := new(persistence)
	err := persist.LoadFile(persistMetadata, p, filepath.Join(h.persistDir, "settings.json"))
	if err != nil {
		return err
	}

	// Consensus Tracking.
	h.blockHeight = p.BlockHeight
	h.recentChange = p.RecentChange

	// Host Identity.
	h.netAddress = p.NetAddress
	h.publicKey = p.PublicKey
	h.secretKey = p.SecretKey

	// Copy over the file management. The space remaining is recalculated from
	// disk instead of being saved, to maximize the potential usefulness of
	// restarting Sia as a means of eliminating unkonwn errors.
	h.fileCounter = p.FileCounter
	h.spaceRemaining = p.Settings.TotalStorage
	h.loadObligations(p.Obligations)

	// Copy over statistics.
	h.revenue = p.Revenue
	h.lostRevenue = p.LostRevenue

	// Copy over rpc tracking.
	atomic.StoreUint64(&h.atomicErroredCalls, p.ErroredCalls)
	atomic.StoreUint64(&h.atomicMalformedCalls, p.MalformedCalls)
	atomic.StoreUint64(&h.atomicDownloadCalls, p.DownloadCalls)
	atomic.StoreUint64(&h.atomicRenewCalls, p.RenewCalls)
	atomic.StoreUint64(&h.atomicReviseCalls, p.ReviseCalls)
	atomic.StoreUint64(&h.atomicSettingsCalls, p.SettingsCalls)
	atomic.StoreUint64(&h.atomicUploadCalls, p.UploadCalls)

	// Utilities.
	h.settings = p.Settings

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

	// Reduce the window size when testing.
	if build.Release == "testing" {
		h.settings.WindowSize = testingWindowSize
	}

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

// initPersist loads all of the saved host state into the host object. If there
// is no saved state, suitable defaults are chosen instead.
func (h *Host) initPersist() error {
	// Create the perist directory if it does not yet exist.
	err := os.MkdirAll(h.persistDir, 0700)
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
