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

// verifyTransaction checks that the provided transaction matches the provided
// contract terms, and that the Merkle root provided is equal to the merkle
// root of the transaction file contract.
func verifyTransaction(txn consensus.Transaction, terms modules.ContractTerms, merkleRoot crypto.Hash) error {
	// Check that there is only one file contract.
	if len(txn.FileContracts) != 1 {
		return errors.New("transaction should have only one file contract.")
	}
	fc := txn.FileContracts[0]

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

// addCollateral takes a transaction and its contract terms and adds the host
// collateral to the transaction.
func (h *Host) addCollateral(txn consensus.Transaction, terms modules.ContractTerms) (fundedTxn consensus.Transaction, txnID string, err error) {
	// Determine the amount of colletaral the host needs to provide.
	sizeCurrency := consensus.NewCurrency64(terms.FileSize)
	durationCurrency := consensus.NewCurrency64(uint64(terms.Duration))
	collateral := terms.Collateral.Mul(sizeCurrency).Mul(durationCurrency)

	txnID, err = h.wallet.RegisterTransaction(txn)
	if err != nil {
		return
	}
	fundedTxn, err = h.wallet.FundTransaction(txnID, collateral)
	if err != nil {
		return
	}
	return
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
	_, err = encoding.WriteObject(conn, modules.AcceptTermsResponse)
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

	// Data has been sent, read in the unsigned transaction with the file
	// contract.
	var unsignedTxn consensus.Transaction
	err = encoding.ReadObject(conn, &unsignedTxn, maxContractLen)
	if err != nil {
		return
	}

	// Verify that the transaction matches the agreed upon terms, and that the
	// Merkle root in the file contract matches our independently calculated
	// Merkle root.
	err = verifyTransaction(unsignedTxn, terms, merkleRoot)
	if err != nil {
		err = errors.New("transaction does not satisfy terms: " + err.Error())
		encoding.WriteObject(conn, err.Error())
		return
	}

	// Add the collateral to the transaction, but do not sign the transaction.
	collateralTxn, txnID, err := h.addCollateral(unsignedTxn, terms)
	if err != nil {
		return
	}
	_, err = encoding.WriteObject(conn, collateralTxn)
	if err != nil {
		return
	}

	// Read in the renter-signed transaction and check that it matches the
	// previously accepted transaction.
	var signedTxn consensus.Transaction
	err = encoding.ReadObject(conn, &signedTxn, maxContractLen)
	if err != nil {
		return
	}
	if collateralTxn.ID() != signedTxn.ID() {
		err = errors.New("signed transaction does not match the transaction with collateral")
		return
	}

	// Add the signatures from the renter signed transaction, and then sign the
	// transaction, then submit the transaction.
	for _, sig := range signedTxn.Signatures {
		_, _, err = h.wallet.AddSignature(txnID, sig)
		if err != nil {
			return
		}
	}
	fullTxn, err := h.wallet.SignTransaction(txnID, true)
	if err != nil {
		return
	}
	err = h.tpool.AcceptTransaction(fullTxn)
	if err != nil {
		return
	}

	// Add this contract to the host's list of obligations.
	fcid := signedTxn.FileContractID(0)
	fc := signedTxn.FileContracts[0]
	proofHeight := fc.Expiration + StorageProofReorgDepth
	co := contractObligation{
		id:           fcid,
		fileContract: fc,
		path:         path,
	}
	h.mu.Lock()
	h.obligationsByHeight[proofHeight] = append(h.obligationsByHeight[proofHeight], co)
	h.obligationsByID[fcid] = co
	h.mu.Unlock()

	return
}
