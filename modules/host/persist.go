package host

import (
	"os"
	"path/filepath"
	"sync/atomic"

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
	Obligations []*contractObligation

	// Statistics.
	Revenue types.Currency

	// RPC Tracking.
	ErroredCalls   uint64
	MalformedCalls uint64
	DownloadCalls  uint64
	RenewCalls     uint64
	ReviseCalls    uint64
	SettingsCalls  uint64
	UploadCalls    uint64
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
		BlockHeight:  h.blockHeight,
		RecentChange: h.recentChange,
		PublicKey:    h.publicKey,
		SecretKey:    h.secretKey,
		Settings:     h.settings,

		FileCounter: h.fileCounter,
		Obligations: h.getObligations(),

		Revenue: h.revenue,

		ErroredCalls:   atomic.LoadUint64(&h.atomicErroredCalls),
		MalformedCalls: atomic.LoadUint64(&h.atomicMalformedCalls),
		DownloadCalls:  atomic.LoadUint64(&h.atomicDownloadCalls),
		RenewCalls:     atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:    atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:  atomic.LoadUint64(&h.atomicSettingsCalls),
		UploadCalls:    atomic.LoadUint64(&h.atomicUploadCalls),
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

		// Check which action is relevant, and queue the obligation for
		// inspection based on what needs to happen next.
		if !obligation.OriginConfirmed || !obligation.RevisionConfirmed {
			// The transaction that validates the obligation has not been
			// sucessfully put on the blockchain. The status should be
			// rechecked and the transaction resubmitted after 'resbumissionTimeout'
			// blocks.
			resubmissionBlock := h.blockHeight + resubmissionTimeout
			h.actionItems[resubmissionBlock] = append(h.actionItems[resubmissionBlock], obligation)
		} else if !obligation.ProofConfirmed {
			// A storage proof for this file contract has not been sucessfully
			// submitted to the blockchain.
			originFC := obligation.OriginTxn.FileContracts[0]
			revs := obligation.RevisionTxn.FileContractRevisions
			proofHeight := originFC.WindowStart
			if len(revs) > 0 && revs[0].NewWindowStart > proofHeight {
				// Use the revision height if there is a revision that triggers
				// at a later height.
				proofHeight = revs[0].NewWindowStart
			}
			if h.blockHeight > proofHeight {
				// Use the current height plus 1 if the current height is ahead
				// of the window start.
				proofHeight = h.blockHeight + 1
			}
			h.actionItems[proofHeight] = append(h.actionItems[proofHeight], obligation)
		} else {
			// A storage proof for this file has been sucessfully confirmed -
			// wait out the confirmation requirement before deleting the
			// contract.
			purgeHeight := h.blockHeight + confirmationRequirement
			h.actionItems[purgeHeight] = append(h.actionItems[purgeHeight], obligation)
		}

		// update spaceRemaining to account for the storage held by this
		// obligation.
		if len(obligation.RevisionTxn.FileContractRevisions) > 0 {
			h.spaceRemaining -= int64(obligation.RevisionTxn.FileContractRevisions[0].NewFileSize)
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
	h.revenue = p.Revenue

	// Copy over rpc tracking.
	atomic.StoreUint64(&h.atomicErroredCalls, p.ErroredCalls)
	atomic.StoreUint64(&h.atomicMalformedCalls, p.MalformedCalls)
	atomic.StoreUint64(&h.atomicDownloadCalls, p.DownloadCalls)
	atomic.StoreUint64(&h.atomicRenewCalls, p.RenewCalls)
	atomic.StoreUint64(&h.atomicReviseCalls, p.ReviseCalls)
	atomic.StoreUint64(&h.atomicSettingsCalls, p.SettingsCalls)
	atomic.StoreUint64(&h.atomicUploadCalls, p.UploadCalls)

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
