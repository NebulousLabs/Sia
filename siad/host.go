package siad

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	HostAnnouncementPrefix = uint64(1)
	AcceptContractResponse = "accept"
)

type ContractEntry struct {
	ID       siacore.ContractID
	Contract *siacore.FileContract
}

type Host struct {
	Settings HostAnnouncement

	SpaceRemaining uint64

	Files map[hash.Hash]string
	index int

	ForwardContracts  map[siacore.BlockHeight][]ContractEntry
	BackwardContracts map[siacore.BlockHeight][]ContractEntry

	sync.Mutex
}

func CreateHost() *Host {
	return new(Host)
}

// SetHostSettings changes the settings according to the input. Need a setter
// because Environment.host is not exported.
func (e *Environment) SetHostSettings(ha HostAnnouncement) {
	e.host.Settings = ha
}

// Wallet.HostAnnounceSelf() creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (e *Environment) HostAnnounceSelf(freezeVolume siacore.Currency, freezeUnlockHeight siacore.BlockHeight, minerFee siacore.Currency) (t siacore.Transaction, err error) {
	info := e.host.Settings

	// Fund the transaction.
	err = e.wallet.FundTransaction(freezeVolume+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output with the freeze volume.
	freezeConditions := e.wallet.SpendConditions
	freezeConditions.TimeLock = freezeUnlockHeight
	info.FreezeIndex = uint64(len(t.Outputs))
	info.SpendConditions = freezeConditions
	t.Outputs = append(t.Outputs, siacore.Output{Value: freezeVolume, SpendHash: freezeConditions.CoinAddress()})

	// Frozen money can't currently be recovered.
	/*
		num, exists := w.OpenFreezeConditions[freezeUnlockHeight]
		if exists {
			w.OpenFreezeConditions[freezeUnlockHeight] = num + 1
		} else {
			w.OpenFreezeConditions[freezeUnlockHeight] = 1
		}
	*/

	// Add the announcement as arbitrary data.
	prefixBytes := encoding.Marshal(HostAnnouncementPrefix)
	announcementBytes := encoding.Marshal(info)
	t.ArbitraryData = append(prefixBytes, announcementBytes...)

	// Sign the transaction.
	e.wallet.SignTransaction(&t)

	e.AcceptTransaction(t)

	// TODO: Have a different method for setting max filesize.
	e.host.SpaceRemaining = e.host.Settings.MaxFilesize

	// TODO: Have a different method for setting max filesize.
	e.host.SpaceRemaining = e.host.Settings.MaxFilesize

	return
}

// considerContract takes a contract and verifies that the negotiations, such
// as price, tolerance, etc. are all valid within the host settings. If so,
// inputs are added to fund the burn part of the contract fund, then the
// updated contract is signed and returned.
func (e *Environment) considerContract(t siacore.Transaction) (nt siacore.Transaction, err error) {
	// Set the new transaction equal to the old transaction. Pretty sure that
	// go does not allow you to return the same variable that was used as
	// input. We could use a pointer, but that might be a bad idea. This call
	// is happening over the network anyway.
	nt = t

	contractDuration := nt.FileContracts[0].End - nt.FileContracts[0].Start // Duration according to the contract.
	fullDuration := nt.FileContracts[0].End - e.Height()                    // Duration that the host will actually be storing the file.
	fileSize := nt.FileContracts[0].FileSize

	// Check that there is only one file contract.
	if len(nt.FileContracts) != 1 {
		err = errors.New("will not accept a transaction with more than one file contract")
		return
	}

	// Check that the file size listed in the contract is in bounds.
	if fileSize < e.host.Settings.MinFilesize || fileSize > e.host.Settings.MaxFilesize {
		err = errors.New("file is of incorrect size")
		return
	}

	// Check that the duration of the contract is in bounds.
	if fullDuration < e.host.Settings.MinDuration || fullDuration < e.host.Settings.MinDuration {
		err = errors.New("contract duration is out of bounds")
		return
	}

	// Check that challenges will not be happening too frequently or infrequently.
	if nt.FileContracts[0].ChallengeFrequency < e.host.Settings.MaxChallengeFrequency || nt.FileContracts[0].ChallengeFrequency > e.host.Settings.MinChallengeFrequency {
		err = errors.New("challenges frequency is too often")
		return
	}

	// Check that tolerance is acceptible.
	if nt.FileContracts[0].Tolerance < e.host.Settings.MinTolerance {
		err = errors.New("tolerance is too low")
		return
	}

	// Outputs for successful proofs need to go to the correct address.
	if nt.FileContracts[0].ValidProofAddress != e.CoinAddress() {
		err = errors.New("coins are not paying out to correct address")
		return
	}

	// Output for failed proofs needs to be the 0 address.
	emptyAddress := siacore.CoinAddress{}
	if nt.FileContracts[0].MissedProofAddress != emptyAddress {
		err = errors.New("burn payout needs to go to the empty address")
		return
	}

	// Verify that output for failed proofs matches burn.
	requiredBurn := e.host.Settings.Burn * siacore.Currency(fileSize) * siacore.Currency(nt.FileContracts[0].ChallengeFrequency)
	if nt.FileContracts[0].MissedProofPayout > requiredBurn {
		err = errors.New("burn payout is too high for a missed proof.")
		return
	}

	// Verify that the outputs for successful proofs are high enough.
	requiredSize := e.host.Settings.Price * siacore.Currency(fileSize) * siacore.Currency(nt.FileContracts[0].ChallengeFrequency)
	if nt.FileContracts[0].ValidProofPayout < requiredSize {
		err = errors.New("valid proof payout is too low")
		return
	}

	// Verify that the contract fund covers the payout and burn for the whole
	// duration.
	requiredFund := (e.host.Settings.Burn + e.host.Settings.Price) * siacore.Currency(fileSize) * siacore.Currency(contractDuration)
	if nt.FileContracts[0].ContractFund < requiredFund {
		err = errors.New("ContractFund does not cover the entire duration of the contract.")
		return
	}

	// Add some inputs and outputs to the transaction to fund the burn half.
	e.wallet.FundTransaction(e.host.Settings.Burn*siacore.Currency(fileSize)*siacore.Currency(contractDuration), &nt)
	e.wallet.SignTransaction(&nt)

	// Check that the transaction is valid after the host signature.
	e.state.Lock()
	err = e.state.ValidTransaction(nt)
	e.state.Unlock()
	if err != nil {
		err = errors.New("post-verified transaction not valid - most likely a client error, but could be a host error too")
		return
	}

	return
}

// NegotiateContract is an RPC that negotiates a file contract. If the
// negotiation is successful, the file is downloaded and the host begins
// submitting proofs of storage.
func (e *Environment) NegotiateContract(conn net.Conn, data []byte) (err error) {
	// read transaction
	var t siacore.Transaction
	if err = encoding.Unmarshal(data, &t); err != nil {
		return
	}
	// consider contract
	if t, err = e.considerContract(t); err != nil {
		_, err = encoding.WriteObject(conn, err.Error())
		return
	} else if _, err = encoding.WriteObject(conn, AcceptContractResponse); err != nil {
		return
	}
	// read file data
	file, err := os.Create(strconv.Itoa(e.host.index))
	if err != nil {
		return
	}
	defer file.Close()
	_, err = io.Copy(file, conn)
	if err != nil {
		return
	}

	// Check that the file matches the merkle root in the contract.
	_, err = file.Seek(0, 0)
	if err != nil {
		return
	}
	merkleRoot, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(uint64(t.FileContracts[0].FileSize)))
	if err != nil {
		return
	}
	if merkleRoot != t.FileContracts[0].FileMerkleRoot {
		err = errors.New("uploaded file has wrong merkle root")
		return
	}

	// Check that the file arrived in time.
	if e.Height() >= t.FileContracts[0].Start-2 {
		err = errors.New("file not uploaded in time, refusing to go forward with contract.")
		return
	}

	// record filename for later retrieval
	e.host.Files[t.FileContracts[0].FileMerkleRoot] = strconv.Itoa(e.host.index)
	e.host.index++

	// Submit the transaction.
	e.AcceptTransaction(t)

	// Put the contract in a list where the host will be performing proofs of
	// storage.
	e.host.ForwardContracts[t.FileContracts[0].Start+1] = append(e.host.ForwardContracts[t.FileContracts[0].Start+1], ContractEntry{ID: t.FileContractID(0), Contract: &t.FileContracts[0]})

	return
}

// RetrieveFile is an RPC that uploads a specified file to a client.
func (e *Environment) RetrieveFile(conn net.Conn, data []byte) (err error) {
	var merkle hash.Hash
	if err = encoding.Unmarshal(data, &merkle); err != nil {
		return
	}
	filename, ok := e.host.Files[merkle]
	if !ok {
		return errors.New("no record of that file")
	}
	f, err := os.Open(filename)
	if err != nil {
		return
	}

	// transmit file
	_, err = io.Copy(conn, f)
	if err != nil {
		return
	}
	return
}

func (e *Environment) CreateProof(contract siacore.FileContract, contractID siacore.ContractID, stateHeight siacore.BlockHeight) (sp siacore.StorageProof, err error) {
	filename, ok := e.host.Files[contract.FileMerkleRoot]
	if !ok {
		err = errors.New("no record of that file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	numSegments := hash.CalculateSegments(contract.FileSize)
	triggerBlock, err := e.BlockAtHeight(stateHeight)
	if err != nil {
		return
	}
	proofIndex := siacore.ContractProofIndex(contractID, stateHeight, contract, triggerBlock.ID())
	base, hashSet, err := hash.BuildReaderProof(file, numSegments, proofIndex)
	if err != nil {
		return
	}
	sp = siacore.StorageProof{contractID, base, hashSet}
	return
}

// storageProofMaintenance tracks when storage proofs need to be submitted as
// transactions, then creates the proof and submits the transaction.
func (e *Environment) storageProofMaintenance(stateHeight siacore.BlockHeight, rewoundBlocks []siacore.BlockID, appliedBlocks []siacore.BlockID) {
	height := stateHeight - siacore.BlockHeight(len(appliedBlocks))
	var proofs []siacore.StorageProof
	for _ = range rewoundBlocks {
		needActionContracts := e.host.BackwardContracts[height]
		for _, contractEntry := range needActionContracts {
			// Create a proof for this contract.
			proof, err := e.CreateProof(*contractEntry.Contract, contractEntry.ID, height)
			if err != nil {
				panic(err)
			}
			proofs = append(proofs, proof)
		}
		height++
	}

	height = stateHeight - siacore.BlockHeight(len(appliedBlocks))
	for _ = range appliedBlocks {
		needActionContracts := e.host.ForwardContracts[height]
		for _, contractEntry := range needActionContracts {
			// Create a proof for this contract.
			proof, err := e.CreateProof(*contractEntry.Contract, contractEntry.ID, height)
			if err != nil {
				panic(err)
			}
			proofs = append(proofs, proof)

			// Add this contract proof to the backwards contracts list.
			e.host.BackwardContracts[height-2] = append(e.host.BackwardContracts[height-2], contractEntry)

			// Add this contract entry to ForwardContracts windowsize blocks into the future.
			e.host.ForwardContracts[height+contractEntry.Contract.ChallengeFrequency] = append(e.host.ForwardContracts[height+contractEntry.Contract.ChallengeFrequency], contractEntry)
		}
		delete(e.host.ForwardContracts, height)
		height++

	}

	if len(proofs) != 0 {
		txn := siacore.Transaction{
			MinerFees:     []siacore.Currency{10},
			StorageProofs: proofs,
		}
		err := e.wallet.FundTransaction(10, &txn)
		if err != nil {
			panic(err) // panic is not the best move here, but some sort of urgent logging would be good.
		}
		err = e.wallet.SignTransaction(&txn)
		if err != nil {
			panic(err)
		}
		e.AcceptTransaction(txn)
	}
}
