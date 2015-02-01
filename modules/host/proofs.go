package host

import (
	//"errors"
	//"os"
	//"path/filepath"

	"github.com/NebulousLabs/Sia/consensus"
	//"github.com/NebulousLabs/Sia/hash"
)

// ContractEntry houses a single contract with its id - you cannot derive the
// id of a contract without having the transaction. Rather than keep the whole
// transaction, we store only the id.
// TODO: is this needed?
type ContractEntry struct {
	ID       consensus.ContractID
	Contract consensus.FileContract
}

// TODO: Hold off on both storage proofs and deleting files for a few blocks
// after the first possible opportunity to reduce risk of loss due to
// blockchain reorganization.
func (h *Host) threadedConsensusListen(updateChan chan struct{}) {
	for _ = range updateChan {
		h.mu.Lock()

		// Get the blocks since the recent update.
		// TODO: Actually update the host.
		_, _, err := h.state.BlocksSince(h.latestBlock)
		if err != nil {
			// This is a severe error and means that the host has somehow
			// desynchronized. In debug mode, panic, but otherwise grab the
			// most recent block and miss out on a bunch of diffs.
			if consensus.DEBUG {
				panic(err)
			}

			// TODO: Log the error
			h.latestBlock = h.state.CurrentBlock().ID()
		}

		/*
			// Iterate through the diffs and submit storage proofs for any contract we
			// recognize.
			var deletions []consensus.ContractID
			var proofs []consensus.StorageProof
			for _, contractDiff := range importantChanges {
				// Check that the contract belongs to us.
				_, exists := h.contracts[contractDiff.ContractID]
				if !exists {
					continue
				}

				// See if one of our contracts has terminated, and prepare to
				// delete the file if it has.
				if contractDiff.Terminated {
					deletions = append(deletions, contractDiff.ContractID)
				}
				if contractDiff.NewOpenContract.WindowSatisfied {
					continue
				}

				entry := ContractEntry{
					ID:       contractDiff.ContractID,
					Contract: contractDiff.Contract,
				}
				proof, err := h.createStorageProof(entry, h.state.Height())
				if err != nil {
					fmt.Println(err)
					continue
				}
				proofs = append(proofs, proof)
			}

			// Create and submit a transaction for every storage proof.
			for _, proof := range proofs {
				// Create the transaction.
				minerFee := consensus.Currency(10) // TODO: ask wallet.
				id, err := h.wallet.RegisterTransaction(consensus.Transaction{})
				if err != nil {
					fmt.Println("High Priority Error: RegisterTransaction failed:", err)
					continue
				}
				err = h.wallet.FundTransaction(id, minerFee)
				if err != nil {
					fmt.Println("High Priority Error: FundTransaction failed:", err)
					continue
				}
				err = h.wallet.AddMinerFee(id, minerFee)
				if err != nil {
					fmt.Println("High Priority Error: AddMinerFee failed:", err)
					continue
				}
				err = h.wallet.AddStorageProof(id, proof)
				if err != nil {
					fmt.Println("High Priority Error: AddStorageProof failed:", err)
					continue
				}
				transaction, err := h.wallet.SignTransaction(id, true)
				if err != nil {
					fmt.Println("High Priority Error: SignTransaction failed:", err)
					continue
				}

				// Submit the transaction.
				h.transactionChan <- transaction
			}

			// Delete all contracts which have expired.
			for _, contractID := range deletions {
				expiredContract := h.contracts[contractID]

				fullpath := filepath.Join(h.hostDir, expiredContract.filename)
				stat, err := os.Stat(fullpath)
				if err != nil {
					fmt.Println(err)
				}
				err = os.Remove(fullpath)
				h.spaceRemaining += stat.Size()
				if err != nil {
					fmt.Println(err)
				}
				delete(h.contracts, contractID)
			}
		*/

		h.mu.Unlock()
	}
}

// Create a proof of storage for a contract, using the state height to
// determine the random seed. Create proof must be under a host and state lock.
func (h *Host) createStorageProof(entry ContractEntry, heightForProof consensus.BlockHeight) (sp consensus.StorageProof, err error) {
	/*
		// Get the file associated with the contract.
		contractObligation, exists := h.contracts[entry.ID]
		if !exists {
			err = errors.New("no record of that file")
			return
		}
		fullname := filepath.Join(h.hostDir, contractObligation.filename)

		// Open the file.
		file, err := os.Open(fullname)
		if err != nil {
			return
		}
		defer file.Close()

		// Build the proof using the hash library.
		numSegments := hash.CalculateSegments(entry.Contract.FileSize)
		segmentIndex, err := h.state.StorageProofSegment(entry.ID)
		if err != nil {
			return
		}
		base, hashSet, err := hash.BuildReaderProof(file, numSegments, segmentIndex)
		if err != nil {
			return
		}
		sp = consensus.StorageProof{entry.ID, base, hashSet}
	*/
	return
}
