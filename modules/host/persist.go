package host

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// persistence is the data that is kept when the host is restarted.
type persistence struct {
	// RPC Metrics.
	DownloadCalls     uint64
	ErroredCalls      uint64
	FormContractCalls uint64
	RenewCalls        uint64
	ReviseCalls       uint64
	SettingsCalls     uint64
	UnrecognizedCalls uint64

	// Consensus Tracking.
	BlockHeight  types.BlockHeight
	RecentChange modules.ConsensusChangeID

	// Host Identity.
	Announced        bool
	AutoAddress      modules.NetAddress
	FinancialMetrics modules.HostFinancialMetrics
	PublicKey        types.SiaPublicKey
	RevisionNumber   uint64
	SecretKey        crypto.SecretKey
	Settings         modules.HostInternalSettings
	UnlockHash       types.UnlockHash

	// Storage Folders.
	SectorSalt     crypto.Hash
	StorageFolders []*storageFolder
}

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostInternalSettings{
		MaxBatchSize: uint64(defaultMaxBatchSize),
		MaxDuration:  defaultMaxDuration,
		WindowSize:   defaultWindowSize,

		Collateral:            defaultCollateral,
		CollateralBudget:      defaultCollateralBudget,
		MaxCollateralFraction: defaultCollateralFraction,
		MaxCollateral:         defaultMaxCollateral,

		MinimumStoragePrice:           defaultStoragePrice,
		MinimumContractPrice:          defaultContractPrice,
		MinimumDownloadBandwidthPrice: defaultDownloadBandwidthPrice,
		MinimumUploadBandwidthPrice:   defaultUploadBandwidthPrice,
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
	_, err = rand.Read(h.sectorSalt[:])
	if err != nil {
		return err
	}

	// Subscribe to the consensus set.
	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return h.save()
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (h *Host) initDB() error {
	return h.db.Update(func(tx *bolt.Tx) error {
		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketActionItems,
			bucketSectorUsage,
			bucketStorageObligations,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// load extrats the save data from disk and populates the host.
func (h *Host) load() error {
	p := new(persistence)
	err := h.dependencies.loadFile(persistMetadata, p, filepath.Join(h.persistDir, settingsFile))
	if os.IsNotExist(err) {
		// There is no host.json file, set up sane defaults.
		return h.establishDefaults()
	} else if err != nil {
		return err
	}

	// Copy over rpc tracking.
	atomic.StoreUint64(&h.atomicDownloadCalls, p.DownloadCalls)
	atomic.StoreUint64(&h.atomicErroredCalls, p.ErroredCalls)
	atomic.StoreUint64(&h.atomicFormContractCalls, p.FormContractCalls)
	atomic.StoreUint64(&h.atomicRenewCalls, p.RenewCalls)
	atomic.StoreUint64(&h.atomicReviseCalls, p.ReviseCalls)
	atomic.StoreUint64(&h.atomicSettingsCalls, p.SettingsCalls)
	atomic.StoreUint64(&h.atomicUnrecognizedCalls, p.UnrecognizedCalls)

	// Copy over consensus tracking.
	h.blockHeight = p.BlockHeight
	h.recentChange = p.RecentChange

	// Copy over host identity.
	h.announced = p.Announced
	h.autoAddress = p.AutoAddress
	h.financialMetrics = p.FinancialMetrics
	h.publicKey = p.PublicKey
	h.revisionNumber = p.RevisionNumber
	h.secretKey = p.SecretKey
	h.settings = p.Settings
	h.unlockHash = p.UnlockHash

	// Copy over storage folders.
	h.storageFolders = p.StorageFolders
	h.sectorSalt = p.SectorSalt

	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return nil
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	p := persistence{
		// RPC Metrics.
		DownloadCalls:     atomic.LoadUint64(&h.atomicDownloadCalls),
		ErroredCalls:      atomic.LoadUint64(&h.atomicErroredCalls),
		FormContractCalls: atomic.LoadUint64(&h.atomicFormContractCalls),
		RenewCalls:        atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:       atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:     atomic.LoadUint64(&h.atomicSettingsCalls),
		UnrecognizedCalls: atomic.LoadUint64(&h.atomicUnrecognizedCalls),

		// Consensus Tracking.
		BlockHeight:  h.blockHeight,
		RecentChange: h.recentChange,

		// Host Identity.
		Announced:        h.announced,
		AutoAddress:      h.autoAddress,
		FinancialMetrics: h.financialMetrics,
		PublicKey:        h.publicKey,
		RevisionNumber:   h.revisionNumber,
		SecretKey:        h.secretKey,
		Settings:         h.settings,
		UnlockHash:       h.unlockHash,

		// Storage Folders.
		StorageFolders: h.storageFolders,
		SectorSalt:     h.sectorSalt,
	}
	return persist.SaveFile(persistMetadata, p, filepath.Join(h.persistDir, settingsFile))
}
