package proto

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// A Downloader retrieves sectors by calling the download RPC on a host.
// Downloaders are NOT thread- safe; calls to Sector must be serialized.
type Downloader struct {
	host      modules.HostDBEntry
	contract  modules.RenterContract // updated after each revision
	conn      net.Conn
	closeChan chan struct{}

	SaveFn revisionSaver
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *Downloader) Sector(root crypto.Hash) (modules.RenterContract, []byte, error) {
	extendDeadline(hd.conn, modules.NegotiateDownloadTime)
	defer extendDeadline(hd.conn, time.Hour) // reset deadline when finished

	// calculate price
	sectorPrice := hd.host.DownloadBandwidthPrice.Mul64(modules.SectorSize)
	if hd.contract.RenterFunds().Cmp(sectorPrice) < 0 {
		return modules.RenterContract{}, nil, errors.New("contract has insufficient funds to support download")
	}
	// to mitigate small errors (e.g. differing block heights), fudge the
	// price and collateral by 0.2%. This is only applied to hosts above
	// v1.0.1; older hosts use stricter math.
	if build.VersionCmp(hd.host.Version, "1.0.1") > 0 {
		sectorPrice = sectorPrice.MulFloat(1 + hostPriceLeeway)
	}

	// create the download revision
	rev := newDownloadRevision(hd.contract.LastRevision, sectorPrice)

	// initiate download by confirming host settings
	if err := startDownload(hd.conn, hd.host); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Before we continue, save the revision. Unexpected termination (e.g.
	// power failure) during the signature transfer leaves in an ambiguous
	// state: the host may or may not have received the signature, and thus
	// may report either revision as being the most recent. To mitigate this,
	// we save the old revision as a fallback.
	if hd.SaveFn != nil {
		if err := hd.SaveFn(rev, hd.contract.MerkleRoots); err != nil {
			return modules.RenterContract{}, nil, err
		}
	}

	// send download action
	err := encoding.WriteObject(hd.conn, []modules.DownloadAction{{
		MerkleRoot: root,
		Offset:     0,
		Length:     modules.SectorSize,
	}})
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// send the revision to the host for approval
	signedTxn, err := negotiateRevision(hd.conn, rev, hd.contract.SecretKey)
	if err == modules.ErrStopResponse {
		// if host gracefully closed, close our connection as well; this will
		// cause the next download to fail. However, we must delay closing
		// until we've finished downloading the sector.
		defer hd.conn.Close()
	} else if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// read sector data, completing one iteration of the download loop
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
	hd.contract.LastRevision = rev
	hd.contract.LastRevisionTxn = signedTxn
	hd.contract.DownloadSpending = hd.contract.DownloadSpending.Add(sectorPrice)

	return hd.contract, sector, nil
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *Downloader) Close() error {
	extendDeadline(hd.conn, modules.NegotiateSettingsTime)
	// don't care about these errors
	_, _ = verifySettings(hd.conn, hd.host)
	_ = modules.WriteNegotiationStop(hd.conn)
	close(hd.closeChan)
	return hd.conn.Close()
}

// NewDownloader initiates the download request loop with a host, and returns a
// Downloader.
func NewDownloader(host modules.HostDBEntry, contract modules.RenterContract, cancel <-chan struct{}) (*Downloader, error) {
	// check that contract has enough value to support a download
	if len(contract.LastRevision.NewValidProofOutputs) != 2 {
		return nil, errors.New("invalid contract")
	}
	sectorPrice := host.DownloadBandwidthPrice.Mul64(modules.SectorSize)
	if contract.RenterFunds().Cmp(sectorPrice) < 0 {
		return nil, errors.New("contract has insufficient funds to support download")
	}

	// initiate download loop
	conn, err := (&net.Dialer{
		Cancel:  cancel,
		Timeout: 15 * time.Second,
	}).Dial("tcp", string(contract.NetAddress))
	if err != nil {
		return nil, err
	}

	closeChan := make(chan struct{})
	go func() {
		select {
		case <-cancel:
			conn.Close()
		case <-closeChan:
		}
	}()

	// allot 2 minutes for RPC request + revision exchange
	extendDeadline(conn, modules.NegotiateRecentRevisionTime)
	defer extendDeadline(conn, time.Hour)
	if err := encoding.WriteObject(conn, modules.RPCDownload); err != nil {
		conn.Close()
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract); err != nil {
		conn.Close() // TODO: close gracefully if host has entered revision loop
		return nil, err
	}

	// the host is now ready to accept revisions
	return &Downloader{
		contract:  contract,
		host:      host,
		conn:      conn,
		closeChan: closeChan,
	}, nil
}
