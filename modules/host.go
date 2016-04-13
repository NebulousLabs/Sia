package modules

// TODO: Host is probably not correctly tracking the financial metrics, nor is
// it properly tracking the RPC metrics for upload and download bandwidth.

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// HostDir names the directory that contains the host persistence.
	HostDir = "host"
)

type (
	// HostBandwidthLimits set limits on the volume, price, and speed of data
	// that's available to the host. Because different ISPs and setups have
	// different rules governing appropriate limits, and because there's a
	// profit incentive to matching the limits as close as possible, and
	// because there might be intelligent ways to allocate data that require
	// outside information (for example, more data available at night), the
	// limits have been created with the idea that some external process will
	// be able to adjust them constantly to conform to transfer limits and take
	// advantage of low-traffic or low-cost hours.
	HostBandwidthLimits struct {
		DownloadLimitGrowth uint64         `json:"downloadlimitgrowth"` // Bytes per second that get added to the limit for how much download bandwidth the host is allowed to use.
		DownloadLimitCap    uint64         `json:"downloadlimitcap"`    // The maximum size of the limit for how much download bandwidth the host is allowed to use.
		DownloadMinPrice    types.Currency `json:"downloadminprice"`    // The minimum price in Hastings per byte of download bandwidth.
		DownloadSpeedLimit  uint64         `json:"downloadspeedlimit"`  // The maximum download speed for all combined host connections.

		UploadLimitGrwoth uint64         `json:"uploadlimitgrowth"` // Bytes per second that get added to the limit for how much upload bandwidth the host is allowed to use.
		UploadLimitCap    uint64         `json:"uploadlimitcap"`    // The maximum size of the limit for how much upload bandwidth the host is allowed to use.
		UploadMinPrice    types.Currency `json:"uploadminprice"`    // The minimum price in Hastings per byte of download bandwidth.
		UploadSpeedLimit  uint64         `json:"uploadspeedlimit"`  // The maximum upload speed for all combined host connections.
	}

	// HostFinancialMetrics provides statistics on the spendings and earnings
	// of the host.
	HostFinancialMetrics struct {
		DownloadBandwidthRevenue           types.Currency
		LockedStorageCollateral            types.Currency
		LostStorageCollateral              types.Currency
		LostStorageRevenue                 types.Currency
		PotentialStorageRevenue            types.Currency
		StorageRevenue                     types.Currency
		TransactionFeeExpenses             types.Currency // Amount spent on transaction fees total
		UnsubsidizedTransactionFeeExpenses types.Currency // Amount spent on transaction fees that the renter did help pay for.
		UploadBandwidthRevenue             types.Currency
	}

	// HostInternalSettings contains a list of settings that can be changed.
	HostInternalSettings struct {
		AcceptingContracts bool              `json:"acceptingcontracts"`
		MaxBatchSize       uint64            `json:"maxbatchsize"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		NetAddress         NetAddress        `json:"netaddress"`
		WindowSize         types.BlockHeight `json:"windowsize"`

		Collateral         types.Currency `json:"collateral"`
		CollateralFraction types.Currency `json:"collateralfraction"`
		MaxCollateral      types.Currency `json:"maxcollateral"`

		BandwidthLimits               HostBandwidthLimits `json:"bandwidthlimits"`
		MinimumContractPrice          types.Currency      `json:"contractprice"`
		MinimumDownloadBandwidthPrice types.Currency      `json:"minimumdownloadbandwidthprice"`
		MinimumStoragePrice           types.Currency      `json:"storageprice"`
		MinimumUploadBandwidthPrice   types.Currency      `json:"minimumuploadbandwidthprice"`
	}

	// HostRPCMetrics reports the quantity of each type of RPC call that has
	// been made to the host.
	HostRPCMetrics struct {
		DownloadBandwidthConsumed uint64 `json:"downloadbandwidthconsumed"`
		UploadBandwidthConsumed   uint64 `json:"uploadbandwidthconsumed"`

		DownloadCalls     uint64 `json:"downloadcalls"`
		ErrorCalls        uint64 `json:"errorcalls"`
		FormContractCalls uint64 `json:"formcontractcalls"`
		RenewCalls        uint64 `json:"renewcalls"`
		ReviseCalls       uint64 `json:"revisecalls"`
		SettingsCalls     uint64 `json:"settingscalls"`
		UnrecognizedCalls uint64 `json:"unrecognizedcalls"`
	}

	// StorageFolderMetadata contians metadata about a storage folder that is
	// tracked by the storage folder manager.
	StorageFolderMetadata struct {
		Capacity          uint64 `json:"capacity"`
		CapacityRemaining uint64 `json:"capacityremaining"`
		Path              string `json:"path"`

		// Below are statistics about the filesystem. FailedReads and
		// FailedWrites are only incremented if the filesystem is returning
		// errors when operations are being performed. A large number of
		// FailedWrites can indicate that more space has been allocated on a
		// drive than is physically available. A high number of failures can
		// also indicaate disk trouble.
		FailedReads      uint64 `json:"failedreads"`
		FailedWrites     uint64 `json:"failedwrites"`
		SuccessfulReads  uint64 `json:"successfulreads"`
		SuccessfulWrites uint64 `json:"successfulwrites"`
	}

	// StorageFolderManager tracks and manipulates storage folders. Storage
	// folders are used by the host to store the data that they contractually
	// agree to manage.
	StorageFolderManager interface {
		// AddStorageFolder adds a storage folder to the host. The host may not
		// check that there is enough space available on-disk to support as
		// much storage as requested, though the host should gracefully handle
		// running out of storage unexpectedly.
		AddStorageFolder(path string, size uint64) error

		// DeleteSector deletes a sector, meaning that the host will be unable
		// to upload that sector and be unable to provide a storage proof on
		// that sector. This function is not intended to be used, but is
		// available in case a host is compelled by their government to delete
		// a piece of illegal data.
		DeleteSector(sectorRoot crypto.Hash) error

		// RemoveStorageFolder will remove a storage folder from the host. All
		// storage on the folder will be moved to other storage folders,
		// meaning that no data will be lost. If the host is unable to save
		// data, an error will be returned and the operation will be stopped.
		RemoveStorageFolder(index int, force bool) error

		// ResetStorageFolderHealth will reset the health statistics on a
		// storage folder.
		ResetStorageFolderHealth(index int) error

		// ResizeStorageFolder will grow or shrink a storage folder in the
		// host. The host may not check that there is enough space on-disk to
		// support growing the storage folder, but should gracefully handle
		// running out of space unexpectedly. When shrinking a storage folder,
		// any data in the folder that needs to be moved will be placed into
		// other storage folders, meaning that no data will be lost. If the
		// host is unable to migrate the data, an error will be returned and
		// the operation will be stopped.
		ResizeStorageFolder(index int, newSize uint64) error

		// StorageFolders will return a list of storage folders tracked by the
		// host.
		StorageFolders() ([]StorageFolderMetadata, error)
	}

	// A Host can take storage from disk and offer it to the network, managing
	// things such as announcements, settings, and implementing all of the RPCs
	// of the host protocol.
	Host interface {
		// Announce submits a host announcement to the blockchain.
		Announce() error

		// AnnounceAddress submits an announcement using the given address.
		AnnounceAddress(NetAddress) error

		// ConsistencyCheckAndRepair runs a consistency check on the host,
		// looking for places where some combination of disk errors, usage
		// errors, and development errors have led to inconsistencies in the
		// host. In cases where these inconsistencies can be repaired, the
		// repairs are made.
		// TODO: ConsistencyCheckAndRepair() error

		// FinancialMetrics returns the financial statistics of the host.
		// TODO: FinancialMetrics() HostFinancialMetrics

		// NetAddress returns the host's network address
		NetAddress() NetAddress

		// RPCMetrics returns information on the types of RPC calls that have
		// been made to the host.
		RPCMetrics() HostRPCMetrics

		// SetInternalSettings sets the hosting parameters of the host.
		SetInternalSettings(HostInternalSettings) error

		// Settings returns the host's internal settings.
		Settings() HostInternalSettings

		// Close saves the state of the host and stops its listener process.
		Close() error

		StorageFolderManager
	}
)

// BandwidthPriceToConsensus converts a human bandwidth price, having the unit
// 'Siacoins per Terabyte', to a consensus storage price, having the unit
// 'Hastings per Byte'.
func BandwidthPriceToConsensus(siacoinsTB uint64) (hastingsByte types.Currency) {
	hastingsTB := types.NewCurrency64(siacoinsTB).Mul(types.SiacoinPrecision)
	return hastingsTB.Div(types.NewCurrency64(1e12))
}

// BandwidthPriceToHuman converts a consensus bandwidth price, having the unit
// 'Hastings per Byte' to a human bandwidth price, having the unit 'Siacoins
// per Terabyte'.
func BandwidthPriceToHuman(hastingsByte types.Currency) (siacoinsTB uint64, err error) {
	hastingsTB := hastingsByte.Mul(types.NewCurrency64(1e12))
	if hastingsTB.Cmp(types.SiacoinPrecision.Div(types.NewCurrency64(2))) < 0 {
		// The result of the final division is going to be less than 0.5,
		// therefore 0 should be returned.
		return 0, nil
	}
	if hastingsTB.Cmp(types.SiacoinPrecision) < 0 {
		// The result of the final division is going to be greater than or
		// equal to 0.5, but less than 1, therefore 1 should be returned.
		return 1, nil
	}
	return hastingsTB.Div(types.SiacoinPrecision).Uint64()
}

// StoragePriceToConsensus converts a human storage price, having the unit
// 'Siacoins per Month per Terabyte', to a consensus storage price, having the
// unit 'Hastings per Block per Byte'.
func StoragePriceToConsensus(siacoinsMonthTB uint64) (hastingsBlockByte types.Currency) {
	// Perform multiplication first to preserve precision.
	hastingsMonthTB := types.NewCurrency64(siacoinsMonthTB).Mul(types.SiacoinPrecision)
	hastingsBlockTB := hastingsMonthTB.Div(types.NewCurrency64(4320))
	return hastingsBlockTB.Div(types.NewCurrency64(1e12))
}

// StoragePriceToHuman converts a consensus storage price, having the unit
// 'Hastings per Block per Byte', to a human storage price, having the unit
// 'Siacoins per Month per Terabyte'. An error is returned if the result would
// overflow a uint64. If the result is between 0 and 1, the value is rounded to
// the nearest value.
func StoragePriceToHuman(hastingsBlockByte types.Currency) (siacoinsMonthTB uint64, err error) {
	// Perform multiplication first to preserve precision.
	hastingsMonthByte := hastingsBlockByte.Mul(types.NewCurrency64(4320))
	hastingsMonthTB := hastingsMonthByte.Mul(types.NewCurrency64(1e12))
	if hastingsMonthTB.Cmp(types.SiacoinPrecision.Div(types.NewCurrency64(2))) < 0 {
		// The result of the final division is going to be less than 0.5,
		// therefore 0 should be returned.
		return 0, nil
	}
	if hastingsMonthTB.Cmp(types.SiacoinPrecision) < 0 {
		// The result of the final division is going to be greater than or
		// equal to 0.5, but less than 1, therefore 1 should be returned.
		return 1, nil
	}
	return hastingsMonthTB.Div(types.SiacoinPrecision).Uint64()
}
