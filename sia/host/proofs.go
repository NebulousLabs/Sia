package host

import (
	"errors"
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/hash"
)

func (h *Host) consensusListen(updateChan chan consensus.ConsensusChange) {
	for consensusChange := range updateChan {
		fmt.Println(consensusChange)
		// For every contract that we recognize that gets destroyed, mark that contract as inactive

		// For every contract that we recognize that gets created, mark that contract as active.
	}
}

// Create a proof of storage for a contract, using the state height to
// determine the random seed. Create proof must be under a host and state lock.
func (h *Host) createStorageProof(entry ContractEntry, heightForProof consensus.BlockHeight) (sp consensus.StorageProof, err error) {
	// Get the file associated with the contract.
	contractObligation, exists := h.contracts[entry.ID]
	if !exists {
		err = errors.New("no record of that file")
		return
	}
	fullname := h.hostDir + contractObligation.filename

	// Open the file.
	file, err := os.Open(fullname)
	if err != nil {
		return
	}
	defer file.Close()

	// Build the proof using the hash library.
	numSegments := hash.CalculateSegments(entry.Contract.FileSize)
	windowIndex, err := entry.Contract.WindowIndex(heightForProof)
	if err != nil {
		return
	}
	segmentIndex, err := h.state.StorageProofSegmentIndex(entry.ID, windowIndex)
	if err != nil {
		return
	}
	base, hashSet, err := hash.BuildReaderProof(file, numSegments, segmentIndex)
	if err != nil {
		return
	}
	sp = consensus.StorageProof{entry.ID, windowIndex, base, hashSet}
	return
}

/*
// storageProofMaintenance tracks when storage proofs need to be submitted as
// transactions, then creates the proof and submits the transaction.
// storageProofMaintenance must be under a state and host lock.
//
// TODO: Make sure that when a contract terminates, the space is returned to
// the unsold space pool, file is deleted, etc.
//
// TODO: Have some method for pruning the backwards contracts map.
//
// TODO: Make sure that hosts don't need to submit a storage proof for the last
// window.
func (h *Host) storageProofMaintenance(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) {
	// Resubmit any proofs that changed as a result of the rewinding.
	height := initialStateHeight
	var proofs []consensus.StorageProof
	for _ = range rewoundBlocks {
		needActionContracts := h.backwardContracts[height]
		for _, contractEntry := range needActionContracts {
			proof, err := h.createStorageProof(contractEntry, height)
			if err != nil {
				fmt.Println("High Priority Error: storage proof failed:", err)
				continue
			}
			proofs = append(proofs, proof)
		}
		height--
	}

	// Submit any proofs that are triggered as the result of new blocks being added.
	for _ = range appliedBlocks {
		needActionContracts := h.forwardContracts[height]
		for _, contractEntry := range needActionContracts {
			proof, err := h.createStorageProof(contractEntry, height)
			if err != nil {
				fmt.Println("High Priority Error: storage proof failed:", err)
				// TODO: Do something that will have the program try again, or
				// revitalize or whatever.
				continue
			}
			proofs = append(proofs, proof)

			// Add this contract proof to the backwards contracts list.
			h.backwardContracts[height-StorageProofReorgDepth+1] = append(h.backwardContracts[height-StorageProofReorgDepth+1], contractEntry)

			// Add this contract entry to ForwardContracts windowsize blocks
			// into the future if the contract has another window.
			nextProof := height + contractEntry.Contract.ChallengeWindow
			if nextProof < contractEntry.Contract.End {
				h.forwardContracts[nextProof] = append(h.forwardContracts[nextProof], contractEntry)
			} else {
				// Delete the file, etc. ==> Can't do this until we resolve the
				// collision problem.
			}
		}
		delete(h.forwardContracts, height)
		height++
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
}
*/
