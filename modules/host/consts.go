package host

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"time"
)

const (
	// defaultMaxDuration defines the maximum number of blocks into the future
	// that the host will accept for the duration of an incoming file contract
	// obligation. 6 months is chosen because hosts are expected to be
	// long-term entities, and because we want to have a set of hosts that
	// support 6 month contracts when Sia leaves beta.
	defaultMaxDuration = 144 * 30 * 6 // 6 months.

	// fileContractNegotiationTimeout indicates the amount of time that a
	// renter has to negotiate a file contract with the host. A timeout is
	// necessary to limit the impact of DoS attacks.
	fileContractNegotiationTimeout = 120 * time.Second

	// iteratedConnectionTime is the amount of time that is allowed to pass
	// before the host will stop accepting new iterations on an iterated
	// connection.
	iteratedConnectionTime = 1200 * time.Second

	// resubmissionTimeout defines the number of blocks that a host will wait
	// before attempting to resubmit a transaction to the blockchain.
	// Typically, this transaction will contain either a file contract, a file
	// contract revision, or a storage proof.
	resubmissionTimeout = 3
)

var (
	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. The collateral should be considered as
	// an absolute instead of as a percentage, because low prices result in
	// collaterals which may be significant by percentage, but insignificant
	// overall. A default of 25 KS / TB / Month has been chosen, which is 2.5x
	// the default price for storage. The host is expected to put up a
	// significant amount of collateral as a commitment to faithfulness,
	// because this guarantees that the incentives are aligned for the host to
	// keep the data even if the price of siacoin fluctuates, the price of raw
	// storage fluctuates, or the host realizes that there is unexpected
	// opportunity cost in being a host.
	defaultCollateral = types.SiacoinPrecision.Mul64(25e3).Div(modules.BlockBytesPerMonthTerabyte) // 25 KS / TB / Month

	// defaultCollateralBudget defines the maximum number of siacoins that the
	// host is going to allocate towards collateral. 10 million has been chosen
	// as a number that is large, but not so large that someone would be
	// furious for losing access to it for a few weeks.
	defaultCollateralBudget = types.SiacoinPrecision.Mul64(10e6)

	// defaultContractPrice defines the default price of creating a contract
	// with the host. The default is set to 30 siacoins, which the file
	// contract revision can have 15 siacoins put towards it, and the storage
	// proof can have 15 siacoins put towards it.
	defaultContractPrice = types.SiacoinPrecision.Mul64(30) // 30 siacoins

	// defaultDownloadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 10 siacoins per gigabyte, because
	// download bandwidth is expected to be plentiful but also in-demand.
	defaultDownloadBandwidthPrice = types.SiacoinPrecision.Mul64(10e3).Div(modules.BytesPerTerabyte) // 10k SC / TB

	// defaultMaxDownloadBatchSize defines the maximum number of bytes that the
	// host will allow to be requested by a single download request. 17 MiB has
	// been chosen because it's 4 full sectors plus some wiggle room. 17 MiB is
	// a conservative default, most hosts will be fine with a number like 65
	// MiB.
	defaultMaxDownloadBatchSize = 17 * (1 << 20)

	// defaultMaxReviseBatchSize defines the maximum number of bytes that the
	// host will allow to be sent during a single batch update in a revision
	// RPC. 17 MiB has been chosen because it's four full sectors, plus some
	// wiggle room for the extra data or a few delete operations. The whole
	// batch will be held in memory, so the batch size should only be increased
	// substantially if the host has a lot of memory. Additionally, the whole
	// batch is sent in one network connection. Additionally, the renter can
	// steal funds for upload bandwidth all the way out to the size of a batch.
	// 17 MiB is a conservative default, most hosts are likely to be just fine
	// with a number like 65 MiB.
	defaultMaxReviseBatchSize = 17 * (1 << 20)

	// defaultMaxCollateral defines the maximum amount of collateral that the
	// host is comfortable putting into a single file contract. 10e3 is a
	// relatively small file contract, but millions of siacoins could be locked
	// away by only a few hundred file contracts. As the ecosystem matures, it
	// is expected that the safe default for this value will increase quite a
	// bit.
	defaultMaxCollateral = types.SiacoinPrecision.Mul64(10e3)

	// defaultStoragePrice defines the starting price for hosts selling
	// storage. We try to match a number that is both reasonably profitable and
	// reasonably competitive.
	defaultStoragePrice = types.SiacoinPrecision.Mul64(10e3).Div(modules.BlockBytesPerMonthTerabyte) // 10k SC / TB / Month

	// defaultUploadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 1 siacoin per GB, because the host is
	// presumed to have a large amount of downstream bandwidth. Furthermore,
	// the host is typically only downloading data if it is planning to store
	// the data, meaning that the host serves to profit from accepting the
	// data.
	defaultUploadBandwidthPrice = types.SiacoinPrecision.Mul64(1e3).Div(modules.BytesPerTerabyte) // 1 SC / TB

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations. For release builds,
	// the default is set to 144 blocks, or about 1 day. This gives the host
	// flexibility to experience downtime without losing file contracts. The
	// optimal default, especially as the network matures, is probably closer
	// to 36 blocks. An experienced or high powered host should not be
	// frustrated by lost coins due to long periods of downtime.
	defaultWindowSize = func() types.BlockHeight {
		if build.Release == "dev" {
			return 36 // 3.6 minutes.
		}
		if build.Release == "standard" {
			return 144 // 1 day.
		}
		if build.Release == "testing" {
			return 5 // 5 seconds.
		}
		panic("unrecognized release constant in host - defaultWindowSize")
	}()

	// maximumLockedStorageObligations sets the maximum number of storage
	// obligations that are allowed to be locked at a time. The map uses an
	// in-memory lock, but also a locked storage obligation could be reading a
	// whole sector into memory, which could use a bunch of system resources.
	maximumLockedStorageObligations = func() uint64 {
		if build.Release == "dev" {
			return 20
		}
		if build.Release == "standard" {
			return 100
		}
		if build.Release == "testing" {
			return 5
		}
		panic("unrecognized release constant in host - maximumLockedStorageObligations")
	}()

	// revisionSubmissionBuffer describes the number of blocks ahead of time
	// that the host will submit a file contract revision. The host will not
	// accept any more revisions once inside the submission buffer.
	revisionSubmissionBuffer = func() types.BlockHeight {
		if build.Release == "dev" {
			return 20 // About 2 minutes
		}
		if build.Release == "standard" {
			return 288 // 2 days.
		}
		if build.Release == "testing" {
			return 4
		}
		panic("unrecognized release constant in host - revision submission buffer")
	}()

	// storageProofConfirmations determines the number of confirmations for a
	// storage proof that the host will wait before
	storageProofConfirmations = func() int {
		if build.Release == "dev" {
			return 20 // About 2 minutes
		}
		if build.Release == "standard" {
			return 72 // About 12 hours
		}
		if build.Release == "testing" {
			return 3
		}
		panic("unrecognized release constant in host - storageProofConfirmations")
	}()
)

// All of the following variables define the names of buckets used by the host
// in the database.
var (
	// bucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retrieved
	// using the id.
	bucketActionItems = []byte("BucketActionItems")

	// bucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	bucketStorageObligations = []byte("BucketStorageObligations")
)

// init runs a series of sanity checks to verify that the constants have sane
// values.
func init() {
	// The revision submission buffer should be greater than the resubmission
	// timeout, because there should be time to perform resubmission if the
	// first attempt to submit the revision fails.
	if revisionSubmissionBuffer < resubmissionTimeout {
		build.Critical("revision submission buffer needs to be larger than or equal to the resubmission timeout")
	}
}
