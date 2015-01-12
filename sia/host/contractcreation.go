package host

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/sia/components"
)

// ContractEntry houses a single contract with its id - you cannot derive the
// id of a contract without having the transaction. Rather than keep the whole
// transaction, we store only the id.
type ContractEntry struct {
	ID       consensus.ContractID
	Contract consensus.FileContract
}

func (h *Host) nextFilename() string {
	h.fileCounter++
	return h.hostDir + strconv.Itoa(h.fileCounter)
}

// considerContract takes a contract and verifies that the terms such as price
// are all valid within the host settings. If so, inputs are added to fund the
// burn part of the contract fund, then the updated contract is signed and
// returned.
//
// TODO: Need to save the space before the file is uploaded, not register it as
// used after the file is uploaded, otherwise there could be a race condition
// that uses more than the available space.
func (h *Host) considerContract(t consensus.Transaction) (updatedTransaction consensus.Transaction, err error) {
	// These variables are here for convenience.
	contractDuration := t.FileContracts[0].End - t.FileContracts[0].Start // Duration according to the contract.
	fullDuration := t.FileContracts[0].End - h.state.Height()             // Duration that the host will actually be storing the file.
	fileSize := t.FileContracts[0].FileSize

	// Check that there is only one file contract.
	if len(t.FileContracts) != 1 {
		err = errors.New("transaction must have exactly one contract")
		return
	}
	// Check that the file size listed in the contract is in bounds.
	if fileSize < h.announcement.MinFilesize || fileSize > h.announcement.MaxFilesize {
		err = fmt.Errorf("file is of incorrect size - filesize %v, min %v, max %v", fileSize, h.announcement.MinFilesize, h.announcement.MaxFilesize)
		return
	}
	// Check that there is space for the file.
	if fileSize > uint64(h.spaceRemaining) {
		err = errors.New("host is at capacity and can not take more files.")
		return
	}
	// Check that the duration of the contract is in bounds.
	if fullDuration < h.announcement.MinDuration || fullDuration > h.announcement.MaxDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}
	// Check that challenges will not be happening too frequently or infrequently.
	if t.FileContracts[0].ChallengeWindow < h.announcement.MinChallengeWindow || t.FileContracts[0].ChallengeWindow > h.announcement.MaxChallengeWindow {
		err = errors.New("challenges frequency is too often")
		return
	}
	// Check that tolerance is acceptible.
	if t.FileContracts[0].Tolerance < h.announcement.MinTolerance {
		err = errors.New("tolerance is too low")
		return
	}
	// Outputs for successful proofs need to go to the correct address.
	if t.FileContracts[0].ValidProofAddress != h.announcement.CoinAddress {
		err = errors.New("coins are not paying out to correct address")
		return
	}
	// Outputs for successful proofs need to match the price.
	requiredSize := h.announcement.Price * consensus.Currency(fileSize) * consensus.Currency(t.FileContracts[0].ChallengeWindow)
	if t.FileContracts[0].ValidProofPayout < requiredSize {
		err = errors.New("valid proof payout is too low")
		return
	}
	// Output for failed proofs needs to be the 0 address.
	emptyAddress := consensus.CoinAddress{}
	if t.FileContracts[0].MissedProofAddress != emptyAddress {
		err = errors.New("burn payout needs to go to the empty address")
		return
	}
	// Verify that output for failed proofs matches burn.
	maxBurn := h.announcement.Burn * consensus.Currency(fileSize) * consensus.Currency(t.FileContracts[0].ChallengeWindow)
	if t.FileContracts[0].MissedProofPayout > maxBurn {
		err = errors.New("burn payout is too high for a missed proof.")
		return
	}
	// Verify that the contract fund covers the payout and burn for the whole
	// duration.
	requiredFund := (h.announcement.Burn + h.announcement.Price) * consensus.Currency(fileSize) * consensus.Currency(fullDuration)
	if t.FileContracts[0].ContractFund < requiredFund {
		err = errors.New("ContractFund does not cover the entire duration of the contract.")
		return
	}

	// Add enough funds to the transaction to cover the penalty half of the
	// agreement.
	penalty := h.announcement.Burn * consensus.Currency(fileSize) * consensus.Currency(contractDuration)
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = h.wallet.FundTransaction(id, penalty)
	if err != nil {
		// TODO: This leaks that the host is out of money.
		return
	}
	updatedTransaction, err = h.wallet.SignTransaction(id, true)

	return
}

// NegotiateContract is an RPC that negotiates a file contract. If the
// negotiation is successful, the file is downloaded and the host begins
// submitting proofs of storage.
//
// Care is taken not to have any locks in place when network communication is
// happening, nor while any intensive file operations are happening. For this
// reason, all of the locking in this function is done manually. Edit with
// caution, review with caution.
//
// TODO: Split some of this logic into helper functions, which in particular
// will make mutex management easier.
func (h *Host) NegotiateContract(conn net.Conn) (err error) {
	// Read the transaction from the connection.
	var t consensus.Transaction
	if err = encoding.ReadObject(conn, &t, maxContractLen); err != nil {
		return
	}

	// Check that the contained FileContract fits host criteria for taking
	// files, replying with the error if there's a problem.
	h.lock()
	t, err = h.considerContract(t)
	h.unlock()
	if err != nil {
		_, err = encoding.WriteObject(conn, err.Error())
		return
	}
	_, err = encoding.WriteObject(conn, components.AcceptContractResponse)
	if err != nil {
		return
	}

	// Create file.
	h.lock()
	filename := h.nextFilename()
	h.unlock()
	file, err := os.Create(filename)
	if err != nil {
		return
	}
	defer file.Close()

	// If there's an error upon return, delete the file that's been created.
	defer func() {
		if err != nil {
			os.Remove(filename)
		}
	}()

	// Download file contents.
	_, err = io.CopyN(file, conn, int64(t.FileContracts[0].FileSize))
	if err != nil {
		return
	}

	// Check that the file matches the merkle root in the contract.
	_, err = file.Seek(0, 0)
	if err != nil {
		return
	}
	merkleRoot, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(t.FileContracts[0].FileSize))
	if err != nil {
		return
	}
	if merkleRoot != t.FileContracts[0].FileMerkleRoot {
		err = errors.New("uploaded file has wrong merkle root")
		return
	}

	// Network communication is finished, and disk intense operations are
	// finished. We can lock the host for the remainder of the function.
	h.lock()
	defer h.unlock()

	// Check that the file arrived in time.
	if h.state.Height() >= t.FileContracts[0].Start-2 {
		err = errors.New("file not uploaded in time, refusing to go forward with contract")
		return
	}

	// Submit the transaction.
	h.transactionChan <- t

	// Put the contract in a list where the host will be performing proofs of
	// storage.
	co := contractObligation{
		inConsensus: false,
		filename:    filename,
	}
	h.contracts[t.FileContractID(0)] = co
	fmt.Println("Accepted contract")

	return
}
