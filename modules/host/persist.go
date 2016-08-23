package host

import (
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
	DownloadCalls       uint64 `json:"downloadcalls"`
	ErroredCalls        uint64 `json:"erroredcalls"`
	FormContractCalls   uint64 `json:"formcontractcalls"`
	RenewCalls          uint64 `json:"renewcalls"`
	ReviseCalls         uint64 `json:"revisecalls"`
	RecentRevisionCalls uint64 `json:"recentrevisioncalls"`
	SettingsCalls       uint64 `json:"settingscalls"`
	UnrecognizedCalls   uint64 `json:"unrecognizedcalls"`

	// Consensus Tracking.
	BlockHeight  types.BlockHeight         `json:"blockheight"`
	RecentChange modules.ConsensusChangeID `json:"recentchange"`

	// Host Identity.
	Announced        bool                         `json:"announced"`
	AutoAddress      modules.NetAddress           `json:"autoaddress"`
	FinancialMetrics modules.HostFinancialMetrics `json:"financialmetrics"`
	PublicKey        types.SiaPublicKey           `json:"publickey"`
	RevisionNumber   uint64                       `json:"revisionnumber"`
	SecretKey        crypto.SecretKey             `json:"secretkey"`
	Settings         modules.HostInternalSettings `json:"settings"`
	UnlockHash       types.UnlockHash             `json:"unlockhash"`
}

// persistData returns the data in the Host that will be saved to disk.
func (h *Host) persistData() persistence {
	return persistence{
		// RPC Metrics.
		DownloadCalls:       atomic.LoadUint64(&h.atomicDownloadCalls),
		ErroredCalls:        atomic.LoadUint64(&h.atomicErroredCalls),
		FormContractCalls:   atomic.LoadUint64(&h.atomicFormContractCalls),
		RenewCalls:          atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:         atomic.LoadUint64(&h.atomicReviseCalls),
		RecentRevisionCalls: atomic.LoadUint64(&h.atomicRecentRevisionCalls),
		SettingsCalls:       atomic.LoadUint64(&h.atomicSettingsCalls),
		UnrecognizedCalls:   atomic.LoadUint64(&h.atomicUnrecognizedCalls),

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
	}
}

// establishDefaults configures the default settings for the host, overwriting
// any existing settings.
func (h *Host) establishDefaults() error {
	// Configure the settings object.
	h.settings = modules.HostInternalSettings{
		MaxDownloadBatchSize: uint64(defaultMaxDownloadBatchSize),
		MaxDuration:          defaultMaxDuration,
		MaxReviseBatchSize:   uint64(defaultMaxReviseBatchSize),
		WindowSize:           defaultWindowSize,

		Collateral:       defaultCollateral,
		CollateralBudget: defaultCollateralBudget,
		MaxCollateral:    defaultMaxCollateral,

		MinStoragePrice:           defaultStoragePrice,
		MinContractPrice:          defaultContractPrice,
		MinDownloadBandwidthPrice: defaultDownloadBandwidthPrice,
		MinUploadBandwidthPrice:   defaultUploadBandwidthPrice,
	}

	// Generate signing key, for revising contracts.
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		return err
	}
	h.secretKey = sk
	h.publicKey = types.Ed25519PublicKey(pk)

	// Subscribe to the consensus set.
	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return nil
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (h *Host) initDB() (err error) {
	// Open the host's database and set up the stop function to close it.
	h.db, err = h.dependencies.openDatabase(dbMetadata, filepath.Join(h.persistDir, dbFilename))
	if err != nil {
		return err
	}
	h.tg.AfterStop(func() {
		err = h.db.Close()
		if err != nil {
			h.log.Println("Could not close the database:", err)
		}
	})

	return h.db.Update(func(tx *bolt.Tx) error {
		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketActionItems,
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

// load loads the Hosts's persistent data from disk.
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
	atomic.StoreUint64(&h.atomicRecentRevisionCalls, p.RecentRevisionCalls)
	atomic.StoreUint64(&h.atomicSettingsCalls, p.SettingsCalls)
	atomic.StoreUint64(&h.atomicUnrecognizedCalls, p.UnrecognizedCalls)

	// Copy over consensus tracking.
	h.blockHeight = p.BlockHeight
	h.recentChange = p.RecentChange

	// Copy over host identity.
	h.announced = p.Announced
	h.autoAddress = p.AutoAddress
	if err := p.AutoAddress.IsValid(); err != nil {
		h.log.Printf("WARN: AutoAddress '%v' loaded from persist is invalid: %v", p.AutoAddress, err)
		h.autoAddress = ""
	}
	h.financialMetrics = p.FinancialMetrics
	h.publicKey = p.PublicKey
	h.revisionNumber = p.RevisionNumber
	h.secretKey = p.SecretKey
	h.settings = p.Settings
	if err := p.Settings.NetAddress.IsValid(); err != nil {
		h.log.Printf("WARN: NetAddress '%v' loaded from persist is invalid: %v", p.Settings.NetAddress, err)
		h.settings.NetAddress = ""
	}
	h.unlockHash = p.UnlockHash

	// Get the number of storage obligations by looking at the storage
	// obligation database.
	err = h.db.View(func(tx *bolt.Tx) error {
		h.financialMetrics.ContractCount = uint64(tx.Bucket(bucketStorageObligations).Stats().KeyN)
		return nil
	})
	if err != nil {
		return err
	}

	// COMPAT v1.0.0
	//
	// Load compatibility fields which may have data leftover. This call should
	// only be relevant the first time the user loads the host after upgrading
	// from v0.6.0 to v1.0.0.
	err = h.loadCompat(p)
	if err != nil {
		return err
	}

	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return nil
}

// loadCompat loads fields that have changed names or otherwise broken
// compatibility with previous versions, enabling users to upgrade without
// unexpected loss of data.
//
// COMPAT v1.0.0
//
// A spelling error in pre-1.0 versions means that, if this is the first time
// running after an upgrade, the misspelled field needs to be transfered over.
func (h *Host) loadCompat(p *persistence) error {
	var compatPersistence struct {
		FinancialMetrics struct {
			PotentialStorageRevenue types.Currency `json:"potentialerevenue"`
		}
		Settings struct {
			MinContractPrice          types.Currency `json:"contractprice"`
			MinDownloadBandwidthPrice types.Currency `json:"minimumdownloadbandwidthprice"`
			MinStoragePrice           types.Currency `json:"storageprice"`
			MinUploadBandwidthPrice   types.Currency `json:"minimumuploadbandwidthprice"`
		}
	}
	err := h.dependencies.loadFile(persistMetadata, &compatPersistence, filepath.Join(h.persistDir, settingsFile))
	if err != nil {
		return err
	}
	// Load the compat values, but only if the compat values are non-zero and
	// the real values are zero.
	if !compatPersistence.FinancialMetrics.PotentialStorageRevenue.IsZero() && p.FinancialMetrics.PotentialStorageRevenue.IsZero() {
		h.financialMetrics.PotentialStorageRevenue = compatPersistence.FinancialMetrics.PotentialStorageRevenue
	}
	if !compatPersistence.Settings.MinContractPrice.IsZero() && p.Settings.MinContractPrice.IsZero() {
		h.settings.MinContractPrice = compatPersistence.Settings.MinContractPrice
	}
	if !compatPersistence.Settings.MinDownloadBandwidthPrice.IsZero() && p.Settings.MinDownloadBandwidthPrice.IsZero() {
		h.settings.MinDownloadBandwidthPrice = compatPersistence.Settings.MinDownloadBandwidthPrice
	}
	if !compatPersistence.Settings.MinStoragePrice.IsZero() && p.Settings.MinStoragePrice.IsZero() {
		h.settings.MinStoragePrice = compatPersistence.Settings.MinStoragePrice
	}
	if !compatPersistence.Settings.MinUploadBandwidthPrice.IsZero() && p.Settings.MinUploadBandwidthPrice.IsZero() {
		h.settings.MinUploadBandwidthPrice = compatPersistence.Settings.MinUploadBandwidthPrice
	}
	return nil
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	return persist.SaveFile(persistMetadata, h.persistData(), filepath.Join(h.persistDir, settingsFile))
}

// saveSync stores all of the persist data to disk and then syncs to disk.
func (h *Host) saveSync() error {
	return persist.SaveFileSync(persistMetadata, h.persistData(), filepath.Join(h.persistDir, settingsFile))
}
