package proto

import (
	"net"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

// A Downloader retrieves sectors by calling the download RPC on a host.
// Downloaders are NOT thread- safe; calls to Sector must be serialized.
type Downloader struct {
	closeChan   chan struct{}
	conn        net.Conn
	contractID  types.FileContractID
	contractSet *ContractSet
	deps        modules.Dependencies
	hdb         hostDB
	host        modules.HostDBEntry
	once        sync.Once
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *Downloader) Sector(root crypto.Hash) (_ modules.RenterContract, _ []byte, err error) {
	// Reset deadline when finished.
	defer extendDeadline(hd.conn, time.Hour) // TODO: Constant.

	// Acquire the contract.
	// TODO: why not just lock the SafeContract directly?
	sc, haveContract := hd.contractSet.Acquire(hd.contractID)
	if !haveContract {
		return modules.RenterContract{}, nil, errors.New("contract not present in contract set")
	}
	defer hd.contractSet.Return(sc)
	contract := sc.header // for convenience

	// calculate price
	sectorPrice := hd.host.DownloadBandwidthPrice.Mul64(modules.SectorSize)
	if contract.RenterFunds().Cmp(sectorPrice) < 0 {
		return modules.RenterContract{}, nil, errors.New("contract has insufficient funds to support download")
	}
	// To mitigate small errors (e.g. differing block heights), fudge the
	// price and collateral by 0.2%.
	sectorPrice = sectorPrice.MulFloat(1 + hostPriceLeeway)

	// create the download revision
	rev := newDownloadRevision(contract.LastRevision(), sectorPrice)

	// initiate download by confirming host settings
	extendDeadline(hd.conn, modules.NegotiateSettingsTime)
	if err := startDownload(hd.conn, hd.host); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// record the change we are about to make to the contract. If we lose power
	// mid-revision, this allows us to restore either the pre-revision or
	// post-revision contract.
	walTxn, err := sc.recordDownloadIntent(rev, sectorPrice)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// send download action
	extendDeadline(hd.conn, 2*time.Minute) // TODO: Constant.
	err = encoding.WriteObject(hd.conn, []modules.DownloadAction{{
		MerkleRoot: root,
		Offset:     0,
		Length:     modules.SectorSize,
	}})
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Increase Successful/Failed interactions accordingly
	defer func() {
		if err != nil {
			hd.hdb.IncrementFailedInteractions(contract.HostPublicKey())
			err = errors.Extend(err, modules.ErrHostFault)
		} else if err == nil {
			hd.hdb.IncrementSuccessfulInteractions(contract.HostPublicKey())
		}
	}()

	// Disrupt before sending the signed revision to the host.
	if hd.deps.Disrupt("InterruptDownloadBeforeSendingRevision") {
		return modules.RenterContract{}, nil,
			errors.New("InterruptDownloadBeforeSendingRevision disrupt")
	}

	// send the revision to the host for approval
	extendDeadline(hd.conn, connTimeout)
	signedTxn, err := negotiateRevision(hd.conn, rev, contract.SecretKey)
	if err == modules.ErrStopResponse {
		// if host gracefully closed, close our connection as well; this will
		// cause the next download to fail. However, we must delay closing
		// until we've finished downloading the sector.
		defer hd.conn.Close()
	} else if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Disrupt after sending the signed revision to the host.
	if hd.deps.Disrupt("InterruptDownloadAfterSendingRevision") {
		return modules.RenterContract{}, nil,
			errors.New("InterruptDownloadAfterSendingRevision disrupt")
	}

	// read sector data, completing one iteration of the download loop
	extendDeadline(hd.conn, modules.NegotiateDownloadTime)
	var sectors [][]byte
	if err := encoding.ReadObject(hd.conn, &sectors, modules.SectorSize+16); err != nil {
		return modules.RenterContract{}, nil, err
	} else if len(sectors) != 1 {
		return modules.RenterContract{}, nil, errors.New("host did not send enough sectors")
	}
	sector := sectors[0]
	if uint64(len(sector)) != modules.SectorSize {
		return modules.RenterContract{}, nil, errors.New("host did not send enough sector data")
	} else if crypto.MerkleRoot(sector) != root {
		return modules.RenterContract{}, nil, errors.New("host sent bad sector data")
	}

	// update contract and metrics
	if err := sc.commitDownload(walTxn, signedTxn, sectorPrice); err != nil {
		return modules.RenterContract{}, nil, err
	}

	return sc.Metadata(), sector, nil
}

// shutdown terminates the revision loop and signals the goroutine spawned in
// NewDownloader to return.
func (hd *Downloader) shutdown() {
	extendDeadline(hd.conn, modules.NegotiateSettingsTime)
	// don't care about these errors
	_, _ = verifySettings(hd.conn, hd.host)
	_ = modules.WriteNegotiationStop(hd.conn)
	close(hd.closeChan)
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *Downloader) Close() error {
	// using once ensures that Close is idempotent
	hd.once.Do(hd.shutdown)
	return hd.conn.Close()
}

// NewDownloader initiates the download request loop with a host, and returns a
// Downloader.
func (cs *ContractSet) NewDownloader(host modules.HostDBEntry, id types.FileContractID, hdb hostDB, cancel <-chan struct{}) (_ *Downloader, err error) {
	sc, ok := cs.Acquire(id)
	if !ok {
		return nil, errors.New("invalid contract")
	}
	defer cs.Return(sc)
	contract := sc.header

	// check that contract has enough value to support a download
	sectorPrice := host.DownloadBandwidthPrice.Mul64(modules.SectorSize)
	if contract.RenterFunds().Cmp(sectorPrice) < 0 {
		return nil, errors.New("contract has insufficient funds to support download")
	}

	// Increase Successful/Failed interactions accordingly
	defer func() {
		// A revision mismatch might not be the host's fault.
		if err != nil && !IsRevisionMismatch(err) {
			hdb.IncrementFailedInteractions(contract.HostPublicKey())
			err = errors.Extend(err, modules.ErrHostFault)
		} else if err == nil {
			hdb.IncrementSuccessfulInteractions(contract.HostPublicKey())
		}
	}()

	conn, closeChan, err := initiateRevisionLoop(host, contract, modules.RPCDownload, cancel, cs.rl)
	if IsRevisionMismatch(err) && len(sc.unappliedTxns) > 0 {
		// we have desynced from the host. If we have unapplied updates from the
		// WAL, try applying them.
		conn, closeChan, err = initiateRevisionLoop(host, sc.unappliedHeader(), modules.RPCDownload, cancel, cs.rl)
		if err != nil {
			return nil, err
		}
		// applying the updates was successful; commit them to disk
		if err := sc.commitTxns(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	// if we succeeded, we can safely discard the unappliedTxns
	for _, txn := range sc.unappliedTxns {
		txn.SignalUpdatesApplied()
	}
	sc.unappliedTxns = nil

	// the host is now ready to accept revisions
	return &Downloader{
		contractID:  id,
		contractSet: cs,
		host:        host,
		conn:        conn,
		closeChan:   closeChan,
		deps:        cs.deps,
		hdb:         hdb,
	}, nil
}
