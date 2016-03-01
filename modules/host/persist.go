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
	// settingsFile is the name of the file that stores host settings.
	settingsFile = "settings.json"
	// logFile establishes the name of the file that gets used for logging.
	logFile = modules.HostDir + ".log"
)

// persistMetadata is the header that gets written to the persist file, and is
// used to recognize other persist files.
var persistMetadata = persist.Metadata{
	Header:  "Sia Host",
	Version: "0.5.2",
}

// persistence is the data that is kept when the host is restarted.
type persistence struct {
	// RPC Metrics.
	ErroredCalls      uint64
	UnrecognizedCalls uint64
	DownloadCalls     uint64
	RenewCalls        uint64
	ReviseCalls       uint64
	SettingsCalls     uint64
	UploadCalls       uint64

	// Consensus Tracking.
	BlockHeight  types.BlockHeight
	RecentChange modules.ConsensusChangeID

	// Host Identity.
	NetAddress modules.NetAddress
	PublicKey  types.SiaPublicKey
	SecretKey  crypto.SecretKey
	SectorSalt crypto.Hash

	// Storage Folders.
	StorageFolders []*storageFolder

	// Financial Metrics.
	DownloadBandwidthRevenue         types.Currency
	LockedStorageCollateral          types.Currency
	LostStorageCollateral            types.Currency
	LostStorageRevenue               types.Currency
	PotentialStorageRevenue          types.Currency
	StorageRevenue                   types.Currency
	TransactionFeeExpenses           types.Currency
	SubsidizedTransactionFeeExpenses types.Currency
	UploadBandwidthRevenue           types.Currency

	// Utilities.
	Settings modules.HostSettings
}

// save stores all of the persist data to disk.
func (h *Host) save() error {
	p := persistence{
		// RPC Metrics.
		ErroredCalls:      atomic.LoadUint64(&h.atomicErroredCalls),
		UnrecognizedCalls: atomic.LoadUint64(&h.atomicUnrecognizedCalls),
		DownloadCalls:     atomic.LoadUint64(&h.atomicDownloadCalls),
		RenewCalls:        atomic.LoadUint64(&h.atomicRenewCalls),
		ReviseCalls:       atomic.LoadUint64(&h.atomicReviseCalls),
		SettingsCalls:     atomic.LoadUint64(&h.atomicSettingsCalls),
		UploadCalls:       atomic.LoadUint64(&h.atomicUploadCalls),

		// Consensus Tracking.
		BlockHeight:  h.blockHeight,
		RecentChange: h.recentChange,

		// Host Identity.
		NetAddress: h.netAddress,
		PublicKey:  h.publicKey,
		SecretKey:  h.secretKey,
		SectorSalt: h.sectorSalt,

		// Storage Folders.
		StorageFolders: h.storageFolders,

		// Financial Metrics.
		DownloadBandwidthRevenue:         h.downloadBandwidthRevenue,
		LockedStorageCollateral:          h.lockedStorageCollateral,
		LostStorageCollateral:            h.lostStorageCollateral,
		LostStorageRevenue:               h.lostStorageRevenue,
		PotentialStorageRevenue:          h.potentialStorageRevenue,
		StorageRevenue:                   h.storageRevenue,
		TransactionFeeExpenses:           h.transactionFeeExpenses,
		SubsidizedTransactionFeeExpenses: h.subsidizedTransactionFeeExpenses,
		UploadBandwidthRevenue:           h.uploadBandwidthRevenue,

		// Utilities.
		Settings: h.settings,
	}
	return persist.SaveFile(persistMetadata, p, filepath.Join(h.persistDir, settingsFile))
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
	atomic.StoreUint64(&h.atomicErroredCalls, p.ErroredCalls)
	atomic.StoreUint64(&h.atomicUnrecognizedCalls, p.UnrecognizedCalls)
	atomic.StoreUint64(&h.atomicDownloadCalls, p.DownloadCalls)
	atomic.StoreUint64(&h.atomicRenewCalls, p.RenewCalls)
	atomic.StoreUint64(&h.atomicReviseCalls, p.ReviseCalls)
	atomic.StoreUint64(&h.atomicSettingsCalls, p.SettingsCalls)
	atomic.StoreUint64(&h.atomicUploadCalls, p.UploadCalls)

	// Copy over consensus tracking.
	h.blockHeight = p.BlockHeight
	h.recentChange = p.RecentChange

	// Copy over host identity.
	h.netAddress = p.NetAddress
	h.publicKey = p.PublicKey
	h.secretKey = p.SecretKey
	h.sectorSalt = p.SectorSalt

	// Copy over storage folders.
	h.storageFolders = p.StorageFolders

	// Copy over financial metrics.
	h.downloadBandwidthRevenue = p.DownloadBandwidthRevenue
	h.lockedStorageCollateral = p.LockedStorageCollateral
	h.lostStorageCollateral = p.LostStorageCollateral
	h.lostStorageRevenue = p.LostStorageRevenue
	h.potentialStorageRevenue = p.PotentialStorageRevenue
	h.storageRevenue = p.StorageRevenue
	h.transactionFeeExpenses = p.TransactionFeeExpenses
	h.subsidizedTransactionFeeExpenses = p.SubsidizedTransactionFeeExpenses
	h.uploadBandwidthRevenue = p.UploadBandwidthRevenue

	// Utilities.
	h.settings = p.Settings

	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}
	return nil
}
