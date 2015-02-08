package host

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	HostCapacityErr = errors.New("host is at capacity and can not take more files")
)

// allocate allocates space for a file and creates it on disk.
func (h *Host) allocate(filesize uint64) (file *os.File, path string, err error) {
	h.spaceRemaining -= int64(filesize)
	h.fileCounter++
	path = filepath.Join(h.hostDir, strconv.Itoa(h.fileCounter))
	file, err = os.Create(path)
	return
}

// deallocate deletes a file and restores its allocated space.
func (h *Host) deallocate(filesize uint64, path string) {
	os.Remove(path)
	h.spaceRemaining += int64(filesize)
}

// considerTerms checks that the terms of a potential file contract fall
// within acceptable bounds, as defined by the host.
func (h *Host) considerTerms(terms modules.ContractTerms) error {
	h.state.RLock()
	maxheight := h.state.Height() + 20
	h.state.RUnlock()

	duration := terms.WindowSize * consensus.BlockHeight(terms.NumWindows)

	switch {
	// TODO: check for minheight too?
	case terms.StartHeight > maxheight:
		return errors.New("first window is too far in the future")

	case terms.FileSize < h.MinFilesize || terms.FileSize > h.MaxFilesize:
		return errors.New("file is of incorrect size")

	case terms.FileSize > uint64(h.spaceRemaining):
		return HostCapacityErr

	case duration < h.MinDuration || duration > h.MaxDuration:
		return errors.New("duration is out of bounds")

	case terms.WindowSize < h.MinWindow:
		return errors.New("challenge window is not large enough")

	case terms.ValidProofAddress != h.CoinAddress:
		return errors.New("coins are not paying out to correct address")

	case terms.MissedProofAddress != consensus.ZeroUnlockHash:
		return errors.New("burn payout needs to go to the zero address")

	case terms.Price.Cmp(h.Price) < 0:
		return errors.New("price does not match host settings")

	case terms.Collateral.Cmp(h.Collateral) > 0:
		return errors.New("collateral does not match host settings")
	}

	return nil
}

// verifyContract verifies that the values in the FileContract match the
// ContractTerms agreed upon.
func verifyContract(contract consensus.FileContract, terms modules.ContractTerms, merkleRoot crypto.Hash) error {
	payout := terms.Price
	err := payout.Add(terms.Collateral)
	if err != nil {
		return err
	}

	switch {
	case contract.FileSize != terms.FileSize:
		return errors.New("bad FileSize")

	case contract.Start != terms.StartHeight:
		return errors.New("bad Start")

	case contract.End != terms.StartHeight+(terms.WindowSize*consensus.BlockHeight(terms.NumWindows)):
		return errors.New("bad End")

	case contract.Payout.Cmp(payout) != 0:
		return errors.New("bad Payout")

	case contract.ValidProofUnlockHash != terms.ValidProofAddress:
		return errors.New("bad ValidProofAddress")

	case contract.MissedProofUnlockHash != terms.MissedProofAddress:
		return errors.New("bad MissedProofAddress")

	case contract.FileMerkleRoot != merkleRoot:
		return errors.New("bad FileMerkleRoot")
	}
	return nil
}

// acceptContract adds the host's funds to the contract transaction and
// submits it to the transaction pool. If we encounter an error here, we
// return a HostCapacityError to hide the fact that we're experiencing
// internal problems.
func (h *Host) acceptContract(txn consensus.Transaction) error {
	contract := txn.FileContracts[0]
	duration := uint64(contract.End - contract.Start)

	penalty := h.Collateral
	penalty.Mul(consensus.NewCurrency64(contract.FileSize))
	err := penalty.Mul(consensus.NewCurrency64(duration))
	// TODO: move this check to a different function?
	if err != nil {
		return err
	}

	id, err := h.wallet.RegisterTransaction(txn)
	if err != nil {
		return HostCapacityErr
	}

	err = h.wallet.FundTransaction(id, penalty)
	if err != nil {
		return HostCapacityErr
	}

	signedTxn, err := h.wallet.SignTransaction(id, true)
	if err != nil {
		return HostCapacityErr
	}

	err = h.tpool.AcceptTransaction(signedTxn)
	if err != nil {
		return HostCapacityErr
	}
	return nil
}

// NegotiateContract is an RPC that negotiates a file contract. If the
// negotiation is successful, the file is downloaded and the host begins
// submitting proofs of storage.
//
// Order of events:
//      1. Renter proposes contract terms
//      2. Host accepts or rejects terms
//      3. If host accepts, renter sends file contents
//      4. Renter funds, signs, and sends transaction containing file contract
//      5. Host verifies transaction matches terms
//      6. Host funds, signs, and submits transaction
//
// TODO: This function's error handling isn't safe; reveals too much info to
// the other party.
func (h *Host) NegotiateContract(conn net.Conn) (err error) {
	// Read the contract terms.
	var terms modules.ContractTerms
	err = encoding.ReadObject(conn, &terms, maxContractLen)
	if err != nil {
		return
	}

	// Consider the contract terms. If they are unnacceptable, return an error
	// describing why.
	h.mu.RLock()
	err = h.considerTerms(terms)
	h.mu.RUnlock()
	if err != nil {
		_, err = encoding.WriteObject(conn, err.Error())
		return
	}

	// terms are acceptable; allocate space for file
	h.mu.Lock()
	file, path, err := h.allocate(terms.FileSize)
	h.mu.Unlock()
	if err != nil {
		return
	}
	defer file.Close()

	// rollback everything if something goes wrong
	defer func() {
		if err != nil {
			h.deallocate(terms.FileSize, path)
		}
	}()

	// signal that we are ready to download file
	_, err = encoding.WriteObject(conn, modules.AcceptContractResponse)
	if err != nil {
		return
	}

	// file transfer is going to take a while, so extend the timeout.
	// This assumes a minimum transfer rate of ~1 Mbps
	conn.SetDeadline(time.Now().Add(time.Duration(terms.FileSize) * 8 * time.Microsecond))

	// simultaneously download file and calculate its Merkle root.
	tee := io.TeeReader(
		// use a LimitedReader to ensure we don't read indefinitely
		io.LimitReader(conn, int64(terms.FileSize)),
		// each byte we read from tee will also be written to file
		file,
	)

	merkleRoot, err := crypto.ReaderMerkleRoot(tee, terms.FileSize)
	if err != nil {
		return
	}

	// Read contract transaction.
	var txn consensus.Transaction
	err = encoding.ReadObject(conn, &txn, maxContractLen)
	if err != nil {
		return
	}

	// Ensure transaction contains a file contract
	if len(txn.FileContracts) != 1 {
		err = errors.New("transaction must contain exactly one file contract")
		encoding.WriteObject(conn, err.Error())
		return
	}
	contract := txn.FileContracts[0]

	// Verify that the contract in the transaction matches the agreed upon
	// terms, and that the Merkle root in the contract matches our
	// independently calculated Merkle root.
	err = verifyContract(contract, terms, merkleRoot)
	if err != nil {
		err = errors.New("contract does not satisfy terms: " + err.Error())
		encoding.WriteObject(conn, err.Error())
		return
	}

	// Fund and submit the transaction.
	err = h.acceptContract(txn)
	if err != nil {
		encoding.WriteObject(conn, err.Error())
		return
	}

	// Add this contract to the host's list of obligations.
	id := txn.FileContractID(0)
	h.mu.Lock()
	h.contracts[id] = contractObligation{
		path: path,
	}
	h.mu.Unlock()

	return
}
