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
	switch {
	case terms.FileSize < h.MinFilesize || terms.FileSize > h.MaxFilesize:
		return errors.New("file is of incorrect size")

	case terms.FileSize > uint64(h.spaceRemaining):
		return HostCapacityErr

	case terms.Duration < h.MinDuration || terms.Duration > h.MaxDuration:
		return errors.New("duration is out of bounds")

	case terms.DurationStart >= h.state.Height():
		return errors.New("duration cannot start in the future")

	case terms.WindowSize < h.MinWindow:
		return errors.New("challenge window is not large enough")

	case terms.Price.Cmp(h.Price) < 0:
		return errors.New("price does not match host settings")

	case terms.Collateral.Cmp(h.Collateral) > 0:
		return errors.New("collateral does not match host settings")

	case len(terms.ValidProofOutputs) != 1:
		return errors.New("payment does not match host settings")

	case terms.ValidProofOutputs[0].UnlockHash != h.UnlockHash:
		return errors.New("payment does not match host settings")

	case len(terms.MissedProofOutputs) != 1:
		return errors.New("refund does not match host settings")

	case terms.MissedProofOutputs[0].UnlockHash != consensus.ZeroUnlockHash:
		return errors.New("coins are not paying out to correct address")
	}

	return nil
}

// verifyContract verifies that the values in the FileContract match the
// ContractTerms agreed upon.
func verifyContract(fc consensus.FileContract, terms modules.ContractTerms, merkleRoot crypto.Hash) error {
	// Get the expected payout.
	sizeCurrency := consensus.NewCurrency64(terms.FileSize)
	durationCurrency := consensus.NewCurrency64(uint64(terms.Duration))
	clientCost := terms.Price.Mul(sizeCurrency).Mul(durationCurrency)
	hostCollateral := terms.Collateral.Mul(sizeCurrency).Mul(durationCurrency)
	expectedPayout := clientCost.Add(hostCollateral)

	switch {
	case fc.FileSize != terms.FileSize:
		return errors.New("bad file contract file size")

	case fc.FileMerkleRoot != merkleRoot:
		return errors.New("bad file contract Merkle root")

	case fc.Start != terms.DurationStart+terms.Duration:
		return errors.New("bad file contract start height")

	case fc.Expiration != terms.DurationStart+terms.Duration+terms.WindowSize:
		return errors.New("bad file contract expiration")

	case fc.Payout.Cmp(expectedPayout) != 0:
		return errors.New("bad file contract payout")

	case len(fc.ValidProofOutputs) != 1:
		return errors.New("bad file contract valid proof outputs")

	case fc.ValidProofOutputs[0].UnlockHash != terms.ValidProofOutputs[0].UnlockHash:
		return errors.New("bad file contract valid proof outputs")

	case len(fc.MissedProofOutputs) != 1:
		return errors.New("bad file contract missed proof outputs")

	case fc.MissedProofOutputs[0].UnlockHash != terms.MissedProofOutputs[0].UnlockHash:
		return errors.New("bad file contract missed proof outputs")

	case fc.TerminationHash != consensus.ZeroUnlockHash:
		return errors.New("bad file contract termination hash")
	}
	return nil
}

// acceptContract adds the host's funds to the contract transaction and
// submits it to the transaction pool. If we encounter an error here, we
// return a HostCapacityError to hide the fact that we're experiencing
// internal problems.
func (h *Host) acceptContract(txn consensus.Transaction) error {
	contract := txn.FileContracts[0]
	duration := uint64(contract.Expiration - contract.Start)
	filesizeCost := consensus.NewCurrency64(contract.FileSize)
	durationCost := consensus.NewCurrency64(duration)
	penalty := h.Collateral.Mul(filesizeCost).Mul(durationCost)

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
	// This assumes a minimum transfer rate of ~64 kbps.
	conn.SetDeadline(time.Now().Add(time.Duration(terms.FileSize) * 128 * time.Microsecond))

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
	fc := txn.FileContracts[0]
	proofHeight := fc.Expiration + StorageProofReorgDepth
	co := contractObligation{
		id:           id,
		fileContract: fc,
		path:         path,
	}
	h.mu.Lock()
	h.obligationsByHeight[proofHeight] = append(h.obligationsByHeight[proofHeight], co)
	h.obligationsByID[id] = co
	h.mu.Unlock()

	return
}
