package modules

import (
	"time"

	"github.com/NebulousLabs/Sia/build"
	//"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// AcceptResponse is the response given to an RPC call to indicate
	// acceptance. (Any other string indicates rejection, and describes the
	// reason for rejection.)
	AcceptResponse = "accept"

	// FileContractNegotiationTime defines the amount of time that the renter
	// and host have to negotiate a file contract. The time is set high enough
	// that a node behind Tor has a reasonable chance at making the multiple
	// required round trips to complete the negotiation.
	FileContractNegotiationTime = 360 * time.Second

	// HostDir names the directory that contains the host persistence.
	HostDir = "host"

	// MaxErrorSize indicates the maximum number of bytes that can be used to
	// encode an error being sent during negotiation.
	MaxErrorSize = 256

	// MaxFileContractSetLen determines the maximum allowed size of a
	// transaction set that can be sent when trying to negotiate a file
	// contract. The transaction set will contain all of the unconfirmed
	// dependencies of the file contract, meaning that it can be quite large.
	// The transaction pool's size limit for transaction sets has been chosen
	// as a reasonable guideline for determining what is too large.
	MaxFileContractSetLen = TransactionSetSizeLimit - 1e3
)

var (
	// ActionInsert is the specifier for a RevisionAction that modifies sector
	// data.
	ActionInsert = types.Specifier{'I', 'n', 's', 'e', 'r', 't'}

	// ActionDelete is the specifier for a RevisionAction that deletes a
	// sector.
	ActionDelete = types.Specifier{'D', 'e', 'l', 'e', 't', 'e'}

	// RPCSettings is the specifier for requesting settings from the host.
	RPCSettings = types.Specifier{'S', 'e', 't', 't', 'i', 'n', 'g', 's', 2}

	// RPCFormContract is the specifier for forming a contract with a host.
	RPCFormContract = types.Specifier{'F', 'o', 'r', 'm', 'C', 'o', 'n', 't', 'r', 'a', 'c', 't'}

	// RPCRenew is the specifier to renewing an existing contract.
	RPCRenew = types.Specifier{'R', 'e', 'n', 'e', 'w', 2}

	// RPCRevise is the specifier for revising an existing file contract.
	RPCRevise = types.Specifier{'R', 'e', 'v', 'i', 's', 'e', 2}

	// RPCDownload is the specifier for downloading a file from a host.
	RPCDownload = types.Specifier{'D', 'o', 'w', 'n', 'l', 'o', 'a', 'd', 2}

	// SectorSize defines how large a sector should be in bytes. The sector
	// size needs to be a power of two to be compatible with package
	// merkletree. 4MB has been chosen for the live network because large
	// sectors significantly reduce the tracking overhead experienced by the
	// renter and the host.
	SectorSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 20 // 1 MiB
		}
		if build.Release == "standard" {
			return 1 << 22 // 4 MiB
		}
		if build.Release == "testing" {
			return 1 << 12 // 4 KiB
		}
		panic("unrecognized release constant in host - sectorSize")
	}()
)

var (
	// PrefixHostAnnouncement is used to indicate that a transaction's
	// Arbitrary Data field contains a host announcement. The encoded
	// announcement will follow this prefix.
	PrefixHostAnnouncement = types.Specifier{'H', 'o', 's', 't', 'A', 'n', 'n', 'o', 'u', 'n', 'c', 'e', 'm', 'e', 'n', '2'}
)

type (
	// A RevisionAction is a description of an edit to be performed on a
	// contract.
	RevisionAction struct {
		Type        types.Specifier
		SectorIndex uint64
	}

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

	// HostAnnouncement declares a nodes intent to be a host, providing a net
	// address that can be used to contact the host.
	HostAnnouncement struct {
		IPAddress NetAddress
		PublicKey types.SiaPublicKey
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

	// HostExternalSettings are the parameters advertised by the host. These
	// are the values that the renter will request from the host in order to
	// build its database.
	HostExternalSettings struct {
		AcceptingContracts bool              `json:"acceptingcontracts"`
		MaxBatchSize       uint64            `json:"maxbatchsize"`
		MaxDuration        types.BlockHeight `json:"maxduration"`
		NetAddress         NetAddress        `json:"netaddress"`
		RemainingStorage   uint64            `json:"remainingstorage"`
		SectorSize         uint64            `json:"sectorsize"`
		TotalStorage       uint64            `json:"totalstorage"`
		UnlockHash         types.UnlockHash  `json:"unlockhash"`
		WindowSize         types.BlockHeight `json:"windowsize"`

		// Collateral is the amount of collateral that the host will put up for
		// storage in 'bytes per block', as an assurance to the renter that the
		// host really is committed to keeping the file. But, because the file
		// contract is created with no data available, this does leave the host
		// exposed to an attack by a wealthy renter whereby the renter causes
		// the host to lockup in-advance a bunch of funds that the renter then
		// never uses, meaning the host will not have collateral for other
		// clients.
		//
		// To mitigate the effects of this attack, the host has a collateral
		// fraction and a max collateral. CollateralFraction is a number that
		// gets divided by 1e6 and then represents the ratio of funds that the
		// host is willing to put into the contract relative to the number of
		// funds that the renter put into the contract. For example, if
		// 'CollateralFraction' is set to 1e6 and the renter adds 1 siacoin of
		// funding to the file contract, the host will also add 1 siacoin of
		// funding to the contract. if 'CollateralFraction' is set to 2e6, the
		// host would add 2 siacoins of funding to the contract.
		//
		// MaxCollateral indicates the maximum number of coins that a host is
		// willing to put into a file contract.
		Collateral         types.Currency `json:"collateral"`
		CollateralFraction types.Currency `json:"collateralfraction"`
		MaxCollateral      types.Currency `json:"maxcollateral"`

		ContractPrice          types.Currency `json:"contractprice"`
		DownloadBandwidthPrice types.Currency `json:"downloadbandwidthprice"`
		StoragePrice           types.Currency `json:"storageprice"`
		UploadBandwidthPrice   types.Currency `json:"uploadbandwidthprice"`

		// Because the host has a public key, and settings are signed, and
		// because settings may be MITM'd, settings need a revision number so
		// that a renter can compare multiple sets of settings and determine
		// which is the most recent.
		RevisionNumber uint64 `json:"revisionnumber"`
		Version        string `json:"version"`
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
		// TODO: ForceRemoveStorageFolder(index int) error

		// RemoveStorageFolder will remove a storage folder from the host. All
		// storage on the folder will be moved to other storage folders,
		// meaning that no data will be lost. If the host is unable to save
		// data, an error will be returned and the operation will be stopped.
		RemoveStorageFolder(index int, force bool) error

		// ResetStorageFolderHealth will reset the health statistics on a
		// storage folder.
		// TODO: ResetStorageFolderHealth(index int) error

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
		// TODO: StorageFolders() []StorageFolderMetadata
	}

	// Host can take storage from disk and offer it to the network, managing things
	// such as announcements, settings, and implementing all of the RPCs of the
	// host protocol.
	Host interface {
		// Announce submits a host announcement to the blockchain. After
		// announcing, the host will begin accepting contracts.
		Announce() error

		// AnnounceAddress submits an announcement using the given address.
		AnnounceAddress(NetAddress) error

		// ConsistencyCheckAndRepair runs a consistency check on the host,
		// looking for places where some combination of disk errors, usage
		// errors, and development errors have led to inconsistencies in the
		// host. In cases where these inconsistencies can be repaired, the
		// repairs are made.
		// TODO: ConsistencyCheckAndRepair() error

		// DeleteSector deletes a sector, meaning that the host will be unable
		// to upload that sector and be unable to provide a storage proof if
		// that sector is chosen by the blockchain.
		// TODO: DeleteSector(sectorRoot crypto.Hash) error

		// FileContracts returns a list of file contracts that the host
		// currently has open, along with the volume of data tracked by each
		// file contract.
		// TODO: FileContracts() ([]types.FileContractID, []uint64)

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
