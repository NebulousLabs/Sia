package modules

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// HostDir names the directory that contains the host persistence.
	HostDir = "host"

	// MaxFileContractSetLen determines the maximum allowed size of a
	// transaction set that can be sent when trying to negotiate a file
	// contract. The transaction set will contain all of the unconfirmed
	// dependencies of the file contract, meaning that it can be quite large.
	// The transaction pool's size limit for transaction sets has been chosen
	// as a reasonable guideline for determining what is too large.
	MaxFileContractSetLen = TransactionSetSizeLimit - 1e3
)

var (
	// PrefixHostAnnouncement is used to indicate that a transaction's
	// Arbitrary Data field contains a host announcement. The encoded
	// announcement will follow this prefix.
	PrefixHostAnnouncement = types.Specifier{'H', 'o', 's', 't', 'A', 'n', 'n', 'o', 'u', 'n', 'c', 'e', 'm', 'e', 'n', '2'}
)

type (
	// HostBandwidthLimits set limits on the volume and speed of the uploading
	// and downloading of the host. The limits have no bearings on the other
	// modules. The data limits are in bytes per month, and the speed limits
	// are in bytes per second.
	HostBandwidthLimits struct {
		DownloadDataLimit  uint64
		DownloadSpeedLimit uint64
		UploadDataLimit    uint64
		UploadSpeedLimit   uint64
	}

	// HostAnnouncement declares a nodes intent to be a host, providing a net
	// address that can be used to contact the host.
	HostAnnouncement struct {
		IPAddress NetAddress
		PublicKey crypto.PublicKey
	}

	// HostFinancialMetrics provides statistics on the spendings and earnings
	// of the host.
	HostFinancialMetrics struct {
		DownloadBandwidthRevenue           types.Currency
		LockedCollateral                   types.Currency
		LostCollateral                     types.Currency
		LostRevenue                        types.Currency
		PotentialStorageRevenue            types.Currency
		StorageRevenue                     types.Currency
		TransactionFeeExpenses             types.Currency // Amount spent on transaction fees total
		UnsubsidizedTransactionFeeExpenses types.Currency // Amount spent on transaction fees that the renter did help pay for.
		UploadBandwidthRevenue             types.Currency
	}

	// HostSettings are the parameters advertised by the host. These are the
	// values that the renter will request from the host in order to build its
	// database.
	HostSettings struct {
		AcceptingContracts bool              `json:"acceptingcontracts"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		NetAddress         NetAddress        `json:"netaddress"`
		RemainingStorage   uint64            `json:"remainingstorage"` // Cannot be directly changed.
		SectorSize         uint64            `json:"sectorsize"`       // Currently cannot be changed (future support planned).
		TotalStorage       uint64            `json:"totalstorage"`     // Cannot be directly changed.
		UnlockHash         types.UnlockHash  `json:"unlockhash"`       // Cannot be directly changed.
		WindowSize         types.BlockHeight `json:"windowsize"`

		Collateral             types.Currency `json:"collateral"`
		ContractPrice          types.Currency `json:"contractprice"`
		DownloadBandwidthPrice types.Currency `json:"downloadbandwidthprice"` // The cost for a renter to download something (meaning the host is uploading).
		StoragePrice           types.Currency `json:"storageprice"`
		UploadBandwidthPrice   types.Currency `json:"uploadbandwidthprice"` // The cost for a renter to upload something (meaning the host is downloading).
	}

	// HostRPCMetrics reports the quantity of each type of RPC call that has
	// been made to the host.
	HostRPCMetrics struct {
		DownloadBandwidthConsumed uint64 `json:"downloadbandwidthconsumed"`
		UploadBandwidthConsumed   uint64 `json:"uploadbandwidthconsumed"`

		ErrorCalls        uint64 `json:"errorcalls"` // Calls that resulted in an error.
		UnrecognizedCalls uint64 `json:"unrecognizedcalls"`
		DownloadCalls     uint64 `json:"downloadcalls"`
		RenewCalls        uint64 `json:"renewcalls"`
		ReviseCalls       uint64 `json:"revisecalls"`
		SettingsCalls     uint64 `json:"settingscalls"`
		UploadCalls       uint64 `json:"uploadcalls"`
	}

	// StorageFolderMetadata contians metadata about a storage folder that is
	// tracked by the storage folder manager.
	StorageFolderMetadata struct {
		CapacityRemaining uint64
		Path              string
		TotalCapacity     uint64

		// Successful and unsuccessful operations report the number of
		// successful and unsuccessful disk operations that the storage manager
		// has completed on the storage folder. A large number of unsuccessful
		// operations can indicate that the space allocated for the storage
		// folder is larger than the amount of actual free space on the disk.
		// Things like filesystem overhead can reduce the amount of actual
		// storage available on disk, but should ultimately be less than 1% of
		// the total advertised capacity of a disk. A large number of
		// unsuccessful operations can also indicate that the disk is failing
		// and that it needs to be replaced.
		SuccessfulOperations   uint64
		UnsuccessfulOperations uint64
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

		// ForceRemoveStorageFolder removes a storage folder. The host will try
		// to save the data on the storage folder by moving it to another
		// storage folder, but if there are errors (such as not enough space)
		// the host will forcibly remove the storage folder, discarding the
		// data. This means that the host will be unable to provide storage
		// proofs on the data, and is going to incur penalties.
		ForceRemoveStorageFolder(index int) error

		// RemoveStorageFolder will remove a storage folder from the host. All
		// storage on the folder will be moved to other storage folders,
		// meaning that no data will be lost. If the host is unable to save
		// data, an error will be returned and the operation will be stopped.
		RemoveStorageFolder(index int) error

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
		StorageFolders() []StorageFolderMetadata
	}

	// Host can take storage from disk and offer it to the network, managing things
	// such as announcements, settings, and implementing all of the RPCs of the
	// host protocol.
	Host interface {
		// Announce submits a host announcement to the blockchain.  After
		// announcing, the host will begin accepting contracts.
		Announce(NetAddress) error

		// ConsistencyCheckAndRepair runs a consistency check on the host,
		// looking for places where some combination of disk errors, usage
		// errors, and development errors have led to inconsistencies in the
		// host. In cases where these inconsistencies can be repaired, the
		// repairs are made.
		// TODO: ConsistencyCheckAndRepair() error

		// DeleteSector deletes a sector, meaning that the host will be unable
		// to upload that sector and be unable to provide a storage proof if
		// that sector is chosen by the blockchain.
		DeleteSector(sectorRoot crypto.Hash) error

		// FileContracts returns a list of file contracts that the host
		// currently has open, along with the volume of data tracked by each
		// file contract.
		FileContracts() ([]types.FileContractID, []uint64)

		// FinancialMetrics returns the financial statistics of the host.
		FinancialMetrics() HostFinancialMetrics

		// NetAddress returns the host's network address
		NetAddress() NetAddress

		// RPCMetrics returns information on the types of RPC calls that have
		// been made to the host.
		RPCMetrics() HostRPCMetrics

		// SetBandwidthLimits puts a limit on how much data transfer the host
		// will tolerate. Altruistic limits indicate how much data the host is
		// willing to transfer for free, and priced limits indicate how much
		// data the host is willing to transfer when the host is getting paid.
		SetBandwidthLimits(altruisticLimits, pricedLimits HostBandwidthLimits)

		// SetConfig sets the hosting parameters of the host.
		SetSettings(HostSettings) error

		// Settings returns the host's settings.
		Settings() HostSettings

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
