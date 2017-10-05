package proto

import (
	"sync"

	"github.com/NebulousLabs/Sia/encoding"
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

	// caches (initialized by initCache)
	flatIDs       []types.FileContractID
	flatContracts []modules.RenterContract
	once          sync.Once
}

// initCache populates the flatIDs and flatContracts fields so that repeated
// work can be avoided when calling the IDs and Contracts methods. It must be
// called under lock. initCache should be called by IDs and Contracts if a
// call to Insert, Return, or Delete has invalidated the cache. This is
// managed via a sync.Once.
func (cs *ContractSet) initCache() {
	cs.flatIDs = make([]types.FileContractID, 0, len(cs.contracts))
	for id := range cs.contracts {
		cs.flatIDs = append(cs.flatIDs, id)
	}
	cs.flatContracts = make([]modules.RenterContract, 0, len(cs.contracts))
	for _, sc := range cs.contracts {
		// construct deep copy, sans MerkleRoots
		c := sc.RenterContract
		c.MerkleRoots = nil
		var contractCopy modules.RenterContract
		encoding.Unmarshal(encoding.Marshal(c), &contractCopy)
		cs.flatContracts = append(cs.flatContracts, contractCopy)
	}
}

// Len returns the number of contracts in the set.
func (cs *ContractSet) Len() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.contracts)
}

// IDs returns the FileContractID of each contract in the set. The contracts
// are not locked. The returned slice must not be modified.
func (cs *ContractSet) IDs() []types.FileContractID {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.once.Do(cs.initCache)
	return cs.flatIDs
}

// Contracts returns a copy of each contract in the set. The contracts are not
// locked, because modifying the copy does not modify the original. Certain
// fields, such as the MerkleRoots, may be set to nil to prevent excess
// copying. The returned slice must not be modified.
func (cs *ContractSet) Contracts() []modules.RenterContract {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.once.Do(cs.initCache)
	return cs.flatContracts
}

// Insert adds a new contract to the set. It panics if the contract is already
// in the set.
//
// TODO: this behavior might not be ideal.
func (cs *ContractSet) Insert(contract modules.RenterContract) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, ok := cs.contracts[contract.ID]; ok {
		panic("contract already in set")
	}
	cs.contracts[contract.ID] = &safeContract{RenterContract: contract}
	// reset sync.Once
	cs.once = sync.Once{}
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

// MustAcquire is a convenience function for acquiring contracts that are
// known to be in the set. It panics otherwise.
func (cs *ContractSet) MustAcquire(id types.FileContractID) modules.RenterContract {
	c, ok := cs.Acquire(id)
	if !ok {
		panic("no contract with that id")
	}
	return c
}

// Return returns a locked contract to the set and unlocks it. The contract
// must have been previously acquired by Acquire. If the contract is not
// present in the set, Return panics.
func (cs *ContractSet) Return(contract modules.RenterContract) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	sc, ok := cs.contracts[contract.ID]
	if !ok {
		panic("no contract with that id")
	}
	sc.RenterContract = contract
	cs.contracts[contract.ID] = sc
	sc.mu.Unlock()
	// reset sync.Once
	cs.once = sync.Once{}
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
	// reset sync.Once
	cs.once = sync.Once{}
}

// NewContractSet returns a ContractSet populated with the provided slice of
// RenterContracts, which may be nil.
func NewContractSet(contracts []modules.RenterContract) ContractSet {
	cs := ContractSet{
		contracts: make(map[types.FileContractID]*safeContract),
	}
	for _, c := range contracts {
		cs.contracts[c.ID] = &safeContract{RenterContract: c}
	}
	return cs
}
