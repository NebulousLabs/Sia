package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	// HostDir names the directory that contains the host persistence.
	HostDir = "host"
)

var (
	// BytesPerTerabyte is the conversion rate between bytes and terabytes.
	BytesPerTerabyte = types.NewCurrency64(1e12)

	// BlockBytesPerMonthTerabyte is the conversion rate between block-bytes and month-TB.
	BlockBytesPerMonthTerabyte = BytesPerTerabyte.Mul64(4320)
)

type (
	// HostContract describes an active or historic storage contract that a
	// host has made with a renter.
	HostContract struct {
		// General contract facts.
		ID                types.FileContractID
		SectorRootCount   uint64 // May be larger than the number of sectors the host is storing due to virtual sectors.
		WindowStartHeight types.BlockHeight
		WindowEndHeight   types.BlockHeight

		// Financial metrics for the contract. Revenue is not realized unless
		// the contract succeeds. Risked collateral is lost if the contract
		// fails. Locked collateral has been returned if the status is not
		// 'Contract Unresolved'.
		ContractCost        types.Currency
		LockedCollateral    types.Currency
		DownloadRevenue     types.Currency
		RiskedCollateral    types.Currency
		StorageRevenue      types.Currency
		TransactionFeesPaid types.Currency
		UploadRevenue       types.Currency

		// The confirmation progression of the storage contract.
		FileContractConfirmed         bool
		FileContractRevisionConfirmed bool
		StorageProofConfirmed         bool

		// The ultimate status of the file contract. Only one will be set to
		// true. Equivalent to an enum, but with stronger type safety.
		ContractUnresolved bool // Usually means it's too early to submit a storage proof.
		ContractRejected   bool // The host was unable to get the file contract confirmed on the blockchain.
		ContractSucceeded  bool // The host successfully submitted a storage proof.
		ContractFailed     bool // The host was unable to complete the file contract, and lost money.
	}

	// HostFinancialMetrics provides financial statistics for the host,
	// including money that is locked in contracts. Though verbose, these
	// statistics should provide a clear picture of where the host's money is
	// currently being used. The front end can consolidate stats where desired.
	// Potential revenue refers to revenue that is available in a file
	// contract for which the file contract window has not yet closed.
	HostFinancialMetrics struct {
		// Every time a renter forms a contract with a host, a contract fee is
		// paid by the renter. These stats track the total contract fees.
		ContractCount                 uint64         `json:"contractcount"`
		ContractCompensation          types.Currency `json:"contractcompensation"`
		PotentialContractCompensation types.Currency `json:"potentialcontractcompensation"`

		// Metrics related to storage proofs, collateral, and submitting
		// transactions to the blockchain.
		LockedStorageCollateral types.Currency `json:"lockedstoragecollateral"`
		LostRevenue             types.Currency `json:"lostrevenue"`
		LostStorageCollateral   types.Currency `json:"loststoragecollateral"`
		PotentialStorageRevenue types.Currency `json:"potentialstoragerevenue"`
		RiskedStorageCollateral types.Currency `json:"riskedstoragecollateral"`
		StorageRevenue          types.Currency `json:"storagerevenue"`
		TransactionFeeExpenses  types.Currency `json:"transactionfeeexpenses"`

		// Bandwidth financial metrics.
		DownloadBandwidthRevenue          types.Currency `json:"downloadbandwidthrevenue"`
		PotentialDownloadBandwidthRevenue types.Currency `json:"potentialdownloadbandwidthrevenue"`
		PotentialUploadBandwidthRevenue   types.Currency `json:"potentialuploadbandwidthrevenue"`
		UploadBandwidthRevenue            types.Currency `json:"uploadbandwidthrevenue"`
	}

	// HostInternalSettings contains a list of settings that can be changed.
	HostInternalSettings struct {
		AcceptingContracts   bool              `json:"acceptingcontracts"`
		MaxDownloadBatchSize uint64            `json:"maxdownloadbatchsize"`
		MaxDuration          types.BlockHeight `json:"maxduration"`
		MaxReviseBatchSize   uint64            `json:"maxrevisebatchsize"`
		NetAddress           NetAddress        `json:"netaddress"`
		WindowSize           types.BlockHeight `json:"windowsize"`

		Collateral       types.Currency `json:"collateral"`
		CollateralBudget types.Currency `json:"collateralbudget"`
		MaxCollateral    types.Currency `json:"maxcollateral"`

		MinContractPrice          types.Currency `json:"mincontractprice"`
		MinDownloadBandwidthPrice types.Currency `json:"mindownloadbandwidthprice"`
		MinStoragePrice           types.Currency `json:"minstorageprice"`
		MinUploadBandwidthPrice   types.Currency `json:"minuploadbandwidthprice"`
	}

	// HostNetworkMetrics reports the quantity of each type of RPC call that
	// has been made to the host.
	HostNetworkMetrics struct {
		DownloadCalls     uint64 `json:"downloadcalls"`
		ErrorCalls        uint64 `json:"errorcalls"`
		FormContractCalls uint64 `json:"formcontractcalls"`
		RenewCalls        uint64 `json:"renewcalls"`
		ReviseCalls       uint64 `json:"revisecalls"`
		SettingsCalls     uint64 `json:"settingscalls"`
		UnrecognizedCalls uint64 `json:"unrecognizedcalls"`
	}

	// A Host can take storage from disk and offer it to the network, managing
	// things such as announcements, settings, and implementing all of the RPCs
	// of the host protocol.
	Host interface {
		// Announce submits a host announcement to the blockchain.
		Announce() error

		// AnnounceAddress submits an announcement using the given address.
		AnnounceAddress(NetAddress) error

		// Contracts returns a list of contracts in the host along with their
		// stats.
		Contracts() ([]HostContract, error)

		// ExternalSettings returns the settings of the host as seen by an
		// untrusted node querying the host for settings.
		ExternalSettings() HostExternalSettings

		// FinancialMetrics returns the financial statistics of the host.
		FinancialMetrics() HostFinancialMetrics

		// InternalSettings returns the host's internal settings, including
		// potentially private or sensitive information.
		InternalSettings() HostInternalSettings

		// NetworkMetrics returns information on the types of RPC calls that
		// have been made to the host.
		NetworkMetrics() HostNetworkMetrics

		// SetInternalSettings sets the hosting parameters of the host.
		SetInternalSettings(HostInternalSettings) error

		// The storage manager provides an interface for adding and removing
		// storage folders and data sectors to the host.
		StorageManager
	}
)
