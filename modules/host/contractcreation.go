package host

import (
	"errors"
	// "fmt"
	// "io"
	"net"
	// "os"
	// "path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	// "github.com/NebulousLabs/Sia/hash"
	// "github.com/NebulousLabs/Sia/modules"
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
	if fileSize < h.MinFilesize || fileSize > h.MaxFilesize {
		err = fmt.Errorf("file is of incorrect size - filesize %v, min %v, max %v", fileSize, h.MinFilesize, h.MaxFilesize)
		return
	}
	// Check that there is space for the file.
	if fileSize > uint64(h.spaceRemaining) {
		err = HostCapacityErr
		return
	}
	// Check that the duration of the contract is in bounds.
	if duration < h.MinDuration || duration > h.MaxDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}
	// Check that the window is large enough.
	if window < h.MinWindow {
		err = errors.New("challenge window is not large enough")
		return
	}
	// Outputs for successful proofs need to go to the correct address.
	if contract.ValidProofAddress != h.CoinAddress {
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
	requiredFund := (h.Price + h.Burn) * consensus.Currency(duration) * consensus.Currency(fileSize)
	if contract.Payout != requiredFund {
		err = errors.New("ContractFund does not match the terms of service.")
		return
	}

	// Add enough funds to the transaction to cover the penalty half of the
	// agreement. If we encounter an error here, we return a HostCapacityError to hide the fact that we're experience internal problems.
	penalty := h.Burn * consensus.Currency(fileSize) * consensus.Currency(duration)
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		err = HostCapacityErr
		return
	}
	err = h.wallet.FundTransaction(id, penalty)
	if err != nil {
		err = HostCapacityErr
		return
	}
	updatedTransaction, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		err = HostCapacityErr
		return
	}

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
// Order of events:
//		1. Contract is sent over without signatures, and doesn't need an
//		acurate Merkle hash of the file. It's just for the host and client
//		to confirm that the terms of the contract are correct and make
//		sense.
//
//		2. Host sends a confirmation, along with an indication of how much
//		the client should contribute for payment, and how much is available
//		for burn.
//
//		3. File is sent over and loaded onto the host's disk. Eventually,
//		micropayments will be worked into this situation to pay the host
//		for bandwidth and whatever storage time.
//
//		4. Client sends over the final client version of the contract
//		containing the appropriate price and burn, with signatures and the
//		correct Merkle root hash.
//
//		5. Host compares the contract with the earlier negotiations and
//		confirms that everything is still the same. Host then verifies that
//		the file which was uploaded has a matching hash to the file in the
//		contract. Host will add burn coins and submit the contract to the
//		blockchain.
//
//		6. If the client double-spends, the host will remove the file. If
//		the host holds the transaction hostage and does not submit it to
//		the blockchain, it will become void.
//
// TODO: This function's error handling isn't safe; reveals too much info to the
// other party.
func (h *Host) NegotiateContract(conn net.Conn) (err error) {
	// Read the transaction from the connection.
	var txn consensus.Transaction
	err = encoding.ReadObject(conn, &txn, maxContractLen)
	if err != nil {
		return
	}
	var startBlock consensus.BlockHeight
	err = encoding.ReadObject(conn, &startBlock, 8) // consensus.BlockHeight is a uint64
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

	// Create file.
	h.mu.Lock()
	h.spaceRemaining -= int64(contract.FileSize)
	filename := h.nextFilename()
	path := filepath.Join(h.hostDir, filename)
	h.mu.Unlock()
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	// rollback everything if something goes wrong
	defer func() {
		if err != nil {
			_, err = encoding.WriteObject(conn, err.Error())
			os.Remove(path)
			h.mu.Lock()
			h.fileCounter--
			h.spaceRemaining -= int64(contract.FileSize)
			h.mu.Unlock()
		}
	}()

	// signal that we are ready to download file
	_, err = encoding.WriteObject(conn, modules.AcceptContractResponse)
	if err != nil {
		return
	}

	// Download file contents.
	_, err = io.CopyN(file, conn, int64(contract.FileSize))
	if err != nil {
		return
	}

	// Check that the file matches the Merkle root in the contract.
	_, err = file.Seek(0, 0)
	if err != nil {
		return
	}
	merkleRoot, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(contract.FileSize))
	if err != nil {
		return
	}
	if merkleRoot != contract.FileMerkleRoot {
		err = errors.New("uploaded file has wrong Merkle root")
		return
	}

	// Download file contents.
	_, err = io.CopyN(file, conn, int64(contract.FileSize))
	if err != nil {
		return
	}

	// Put the contract in a list where the host will be performing proofs of
	// storage.
	h.mu.Lock()
	h.contracts[txn.FileContractID(0)] = contractObligation{
		filename: filename,
	}
	h.mu.Unlock()
	//fmt.Println("Accepted contract")

	// Submit the transaction.
	h.state.AcceptTransaction(txn)

	return
}
