package host

import (
	// "errors"
	// "fmt"
	// "io"
	// "net"
	// "os"
	// "strconv"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/sia/components"
)

const (
	AcceptContractResponse = "accept"
	StorageProofReorgDepth = 6 // How many blocks to wait before submitting a storage proof.
	maxContractLen         = 1 << 24
)

type Host struct {
	announcement   components.HostAnnouncement
	spaceRemaining int64
	wallet         components.Wallet

	height          consensus.BlockHeight      // Current height of the state.
	transactionChan chan consensus.Transaction // Can send channels to the state.

	FileCounter int
	Files       map[hash.Hash]string

	ForwardContracts  map[consensus.BlockHeight][]ContractEntry
	BackwardContracts map[consensus.BlockHeight][]ContractEntry

	rwLock sync.RWMutex
}

// New returns an initialized Host.
func New() (h *Host) {
	return &Host{
		Files:             make(map[hash.Hash]string),
		ForwardContracts:  make(map[consensus.BlockHeight][]ContractEntry),
		BackwardContracts: make(map[consensus.BlockHeight][]ContractEntry),
	}
}

// UpdateSettings changes the settings of the host to the input settings.
// SpaceRemaining will be changed accordingly, and will not return an error if
// space remaining goes negative.
func (h *Host) UpdateHostSettings(newSettings components.HostSettings) error {
	h.lock()
	defer h.unlock()

	storageDiff := newSettings.Announcement.TotalStorage - h.announcement.TotalStorage
	h.spaceRemaining += storageDiff

	h.announcement = newSettings.Announcement
	h.height = newSettings.Height
	h.transactionChan = newSettings.TransactionChan
	h.wallet = newSettings.Wallet
	return nil
}

/*
// RetrieveFile is an RPC that uploads a specified file to a client.
func (e *Core) RetrieveFile(conn net.Conn) (err error) {
	// Get the filename.
	var merkle hash.Hash
	if err = encoding.ReadObject(conn, &merkle, hash.HashSize); err != nil {
		return
	}

	// Verify the file exists.
	e.host.RLock()
	filename, exists := e.host.Files[merkle]
	e.host.RUnlock()
	if !exists {
		fmt.Println("RetrieveFile: no record of file with that hash")
		return errors.New("no record of that file")
	}

	// Open the file.
	file, err := os.Open(e.hostDir + filename)
	if err != nil {
		fmt.Println("RetrieveFile:", err)
		return
	}
	defer file.Close()

	// Transmit the file.
	_, err = io.Copy(conn, file)
	if err != nil {
		return
	}

	return
}

// Create a proof of storage for a contract, using the state height to
// determine the random seed. Create proof must be under a host and state lock.
func (e *Core) createStorageProof(contractEntry ContractEntry, stateHeight consensus.BlockHeight) (sp consensus.StorageProof, err error) {
	// Get the file associated with the contract.
	filename, ok := e.host.Files[contractEntry.Contract.FileMerkleRoot]
	if !ok {
		err = errors.New("no record of that file")
	}

	// Open the file.
	file, err := os.Open(e.hostDir + filename)
	if err != nil {
		return
	}
	defer file.Close()

	// Build the proof using the hash library.
	numSegments := hash.CalculateSegments(contractEntry.Contract.FileSize)
	windowIndex, err := contractEntry.Contract.WindowIndex(stateHeight)
	if err != nil {
		return
	}
	segmentIndex, err := e.state.StorageProofSegmentIndex(contractEntry.ID, windowIndex)
	if err != nil {
		return
	}
	base, hashSet, err := hash.BuildReaderProof(file, numSegments, segmentIndex)
	if err != nil {
		return
	}
	sp = consensus.StorageProof{contractEntry.ID, windowIndex, base, hashSet}
	return
}

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
func (e *Core) storageProofMaintenance(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.BlockID, appliedBlocks []consensus.BlockID) {
	// Resubmit any proofs that changed as a result of the rewinding.
	height := initialStateHeight
	var proofs []consensus.StorageProof
	for _ = range rewoundBlocks {
		needActionContracts := e.host.BackwardContracts[height]
		for _, contractEntry := range needActionContracts {
			proof, err := e.createStorageProof(contractEntry, height)
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
		needActionContracts := e.host.ForwardContracts[height]
		for _, contractEntry := range needActionContracts {
			proof, err := e.createStorageProof(contractEntry, height)
			if err != nil {
				fmt.Println("High Priority Error: storage proof failed:", err)
				// TODO: Do something that will have the program try again, or
				// revitalize or whatever.
				continue
			}
			proofs = append(proofs, proof)

			// Add this contract proof to the backwards contracts list.
			e.host.BackwardContracts[height-StorageProofReorgDepth+1] = append(e.host.BackwardContracts[height-StorageProofReorgDepth+1], contractEntry)

			// Add this contract entry to ForwardContracts windowsize blocks
			// into the future if the contract has another window.
			nextProof := height + contractEntry.Contract.ChallengeWindow
			if nextProof < contractEntry.Contract.End {
				e.host.ForwardContracts[nextProof] = append(e.host.ForwardContracts[nextProof], contractEntry)
			} else {
				// Delete the file, etc. ==> Can't do this until we resolve the
				// collision problem.
			}
		}
		delete(e.host.ForwardContracts, height)
		height++
	}

	// Create and submit a transaction for every storage proof.
	for _, proof := range proofs {
		// Create the transaction.
		minerFee := consensus.Currency(10) // TODO: ask wallet.
		id, err := e.wallet.RegisterTransaction(consensus.Transaction{})
		if err != nil {
			fmt.Println("High Priority Error: RegisterTransaction failed:", err)
			continue
		}
		err = e.wallet.FundTransaction(id, minerFee)
		if err != nil {
			fmt.Println("High Priority Error: FundTransaction failed:", err)
			continue
		}
		err = e.wallet.AddMinerFee(id, minerFee)
		if err != nil {
			fmt.Println("High Priority Error: AddMinerFee failed:", err)
			continue
		}
		err = e.wallet.AddStorageProof(id, proof)
		if err != nil {
			fmt.Println("High Priority Error: AddStorageProof failed:", err)
			continue
		}
		transaction, err := e.wallet.SignTransaction(id, true)
		if err != nil {
			fmt.Println("High Priority Error: SignTransaction failed:", err)
			continue
		}

		// Submit the transaction.
		err = e.AcceptTransaction(transaction)
		if err != nil {
			fmt.Println("High Priority Error: SignTransaction failed:", err)
		}
	}
}
*/
