package proto

import (
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A safeContract protects a RenterContract with a mutex.
type safeContract struct {
	modules.RenterContract
	mu sync.Mutex
}

// A ContractSet provides safe concurrent access to a set of contracts. Its
// purpose is to serialize modifications to individual contracts, as well as
// to provide operations on the set as a whole.
type ContractSet struct {
	contracts map[types.FileContractID]*safeContract
	mu        sync.Mutex
}

// Len returns the number of contracts in the set.
func (cs *ContractSet) Len() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.contracts)
}

// IDs returns the FileContractID of each contract in the set. The contracts
// are not locked.
func (cs *ContractSet) IDs() []types.FileContractID {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	ids := make([]types.FileContractID, 0, len(cs.contracts))
	for id := range cs.contracts {
		ids = append(ids, id)
	}
	return ids
}

// Contracts returns a copy of each contract in the set. The contracts are not
// locked. Certain fields, including the MerkleRoots, are set to nil for
// safety reasons.
func (cs *ContractSet) Contracts() []modules.RenterContract {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	contracts := make([]modules.RenterContract, 0, len(cs.contracts))
	for _, sc := range cs.contracts {
		// construct shallow copy, sans MerkleRoots
		c := sc.RenterContract
		c.MerkleRoots = nil
		contracts = append(contracts, c)
	}
	return contracts
}

// Insert adds a new contract to the set. It panics if the contract is already
// in the set.
func (cs *ContractSet) Insert(contract modules.RenterContract) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, ok := cs.contracts[contract.ID]; ok {
		build.Critical("contract already in set")
	}
	cs.contracts[contract.ID] = &safeContract{RenterContract: contract}
}

// Acquire looks up the contract with the specified FileContractID and locks
// it before returning it. If the contract is not present in the set, Acquire
// returns false and a zero-valued RenterContract.
func (cs *ContractSet) Acquire(id types.FileContractID) (modules.RenterContract, bool) {
	cs.mu.Lock()
	sc, ok := cs.contracts[id]
	cs.mu.Unlock()
	if ok {
		sc.mu.Lock()
	}
	return sc.RenterContract, ok
}

// Return returns a locked contract to the set and unlocks it. The contract
// must have been previously acquired by Acquire. If the contract is not
// present in the set, Return panics.
func (cs *ContractSet) Return(contract modules.RenterContract) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	sc, ok := cs.contracts[contract.ID]
	if !ok {
		build.Critical("no contract with that id")
	}
	sc.RenterContract = contract
	cs.contracts[contract.ID] = sc
	sc.mu.Unlock()
}

// Delete removes a contract from the set. The contract must have been
// previously acquired by Acquire. If the contract is not present in the set,
// Delete is a no-op.
func (cs *ContractSet) Delete(contract modules.RenterContract) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	sc, ok := cs.contracts[contract.ID]
	if !ok {
		return
	}
	delete(cs.contracts, contract.ID)
	sc.mu.Unlock()
}

// NewContractSet returns a ContractSet populated with the provided slice of
// RenterContracts, which may be nil.
func NewContractSet(contracts []modules.RenterContract) ContractSet {
	set := make(map[types.FileContractID]*safeContract)
	for _, c := range contracts {
		set[c.ID] = &safeContract{RenterContract: c}
	}
	return ContractSet{
		contracts: set,
	}
}
