package host

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	HostCapacityErr = errors.New("host is at capacity and can not take more files")
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
	return strconv.Itoa(h.fileCounter)
}

// considerContract takes a contract and verifies that the terms such as price
// are all valid within the host settings. If so, inputs are added to fund the
// burn part of the contract fund, then the updated contract is signed and
// returned.
//
// TODO: Need to save the space before the file is uploaded, not register it as
// used after the file is uploaded, otherwise there could be a race condition
// that uses more than the available space.
//
// TODO: Make the host able to parse multiple contracts at once.
func (h *Host) considerContract(t consensus.Transaction, startBlock consensus.BlockHeight) (updatedTransaction consensus.Transaction, err error) {
	// Check that there is exactly one file contract.
	if len(t.FileContracts) != 1 {
		err = errors.New("transaction must have exactly one contract")
		return
	}
	if startBlock > h.state.Height()+20 {
		err = errors.New("startBlock is too far in the future")
		return
	}

	// These variables are here for convenience.
	contract := t.FileContracts[0]
	window := contract.End - contract.Start
	duration := contract.End - startBlock
	fileSize := contract.FileSize

	// Check that the file size listed in the contract is in bounds.
	if fileSize < h.announcement.MinFilesize || fileSize > h.announcement.MaxFilesize {
		err = fmt.Errorf("file is of incorrect size - filesize %v, min %v, max %v", fileSize, h.announcement.MinFilesize, h.announcement.MaxFilesize)
		return
	}
	// Check that there is space for the file.
	if fileSize > uint64(h.spaceRemaining) {
		err = HostCapacityErr
		return
	}
	// Check that the duration of the contract is in bounds.
	if duration < h.announcement.MinDuration || duration > h.announcement.MaxDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}
	// Check that the window is large enough.
	if window < h.announcement.MinWindow {
		err = errors.New("challenge window is not large enough")
		return
	}
	// Outputs for successful proofs need to go to the correct address.
	if contract.ValidProofAddress != h.announcement.CoinAddress {
		err = errors.New("coins are not paying out to correct address")
		return
	}
	// Output for failed proofs needs to be the 0 address.
	emptyAddress := consensus.CoinAddress{}
	if contract.MissedProofAddress != emptyAddress {
		err = errors.New("burn payout needs to go to the empty address")
		return
	}

	// Verify that the contract fund covers the payout and burn for the whole
	// duration.
	requiredFund := (h.announcement.Price + h.announcement.Burn) * consensus.Currency(duration) * consensus.Currency(fileSize)
	if contract.Payout != requiredFund {
		err = errors.New("ContractFund does not match the terms of service.")
		return
	}

	// Add enough funds to the transaction to cover the penalty half of the
	// agreement.
	penalty := h.announcement.Burn * consensus.Currency(fileSize) * consensus.Currency(duration)
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		err = HostCapacityErr // hide the fact that the host is having wallet issues.
		return
	}
	err = h.wallet.FundTransaction(id, penalty)
	if err != nil {
		err = HostCapacityErr // hide the fact that the host is having wallet issues.
		return
	}
	updatedTransaction, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		err = HostCapacityErr // hide the fact that the host is having wallet issues.
		return
	}

	// Update the amount of space the host has for sale.
	h.spaceRemaining -= int64(fileSize)

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
// TODO: Order of events: (not currently followed)
//			1. Contract is sent over without signatures, and doesn't need an
//			acurate merkle hash of the file. It's just for the host and client
//			to confirm that the terms of the contract are correct and make
//			sens.
//
//			2. Host sends a confirmation, along with an indication of how much
//			the client should contribute for payment, and how much is available
//			for burn.
//
//			3. File is sent over and loaded onto the host's disk. Eventually,
//			micropayments will be worked into this situation to pay the host
//			for bandwidth and whatever storage time.
//
//			4. Client sends over the final client version of the contract
//			containing the appropriate price and burn, with signatures and the
//			correct merkle root hash.
//
//			5. Host compares the contract with the earlier negotiations and
//			confirms that everything is still the same. Host then verifies that
//			the file which was uploaded has a matching hash to the file in the
//			contract. Host will add burn dollars and submit the contract to the
//			blockchain.
//
//			6. If the client double-spends, the host will remove the file. If
//			the host holds the transaction hostage and does not submit it to
//			the blockchain, it will become void.
//
// TODO: This functions error handling isn't safe, reveals too much info to the
// other party.
func (h *Host) NegotiateContract(conn net.Conn) (err error) {
	// Read the transaction from the connection.
	var txn consensus.Transaction
	err = encoding.ReadObject(conn, &txn, maxContractLen)
	if err != nil {
		return
	}
	var startBlock consensus.BlockHeight
	err = encoding.ReadObject(conn, &startBlock, maxContractLen) // what should be the maxlen here?
	if err != nil {
		return
	}
	contract := txn.FileContracts[0]

	// Check that the contained FileContract fits host criteria for taking
	// files, replying with the error if there's a problem.
	h.mu.Lock()
	txn, err = h.considerContract(txn, startBlock)
	h.mu.Unlock()
	if err != nil {
		_, err = encoding.WriteObject(conn, err.Error())
		return
	}
	_, err = encoding.WriteObject(conn, modules.AcceptContractResponse)
	if err != nil {
		return
	}

	// Create file.
	h.mu.Lock()
	filename := h.nextFilename()
	fullname := filepath.Join(h.hostDir, filename)
	h.mu.Unlock()
	file, err := os.Create(fullname)
	if err != nil {
		return
	}
	defer file.Close()

	// If there's an error upon return, delete the file that's been created.
	defer func() {
		if err != nil {
			_, err = encoding.WriteObject(conn, err.Error())
			os.Remove(fullname)
			h.mu.Lock()
			h.spaceRemaining -= int64(contract.FileSize)
			h.mu.Unlock()
		}
	}()

	// Download file contents.
	_, err = io.CopyN(file, conn, int64(contract.FileSize))
	if err != nil {
		return
	}

	// Check that the file matches the merkle root in the contract.
	_, err = file.Seek(0, 0)
	if err != nil {
		return
	}
	merkleRoot, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(contract.FileSize))
	if err != nil {
		return
	}
	if merkleRoot != contract.FileMerkleRoot {
		err = errors.New("uploaded file has wrong merkle root")
		return
	}

	// Check that the file arrived in time.
	if h.state.Height() >= contract.Start-2 {
		err = errors.New("file not uploaded in time, refusing to go forward with contract")
		return
	}

	// Put the contract in a list where the host will be performing proofs of
	// storage.
	h.mu.Lock()
	h.contracts[txn.FileContractID(0)] = contractObligation{
		filename: filename,
	}
	h.mu.Unlock()
	fmt.Println("Accepted contract")

	// Submit the transaction.
	h.state.AcceptTransaction(txn)

	return
}
