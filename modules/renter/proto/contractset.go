package proto

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/writeaheadlog"
)

// A ContractSet provides safe concurrent access to a set of contracts. Its
// purpose is to serialize modifications to individual contracts, as well as
// to provide operations on the set as a whole.
type ContractSet struct {
	contracts map[types.FileContractID]*SafeContract
	wal       *writeaheadlog.WAL
	dir       string
	mu        sync.Mutex
}

// Acquire looks up the contract with the specified FileContractID and locks
// it before returning it. If the contract is not present in the set, Acquire
// returns false and a zero-valued RenterContract.
func (cs *ContractSet) Acquire(id types.FileContractID) (*SafeContract, bool) {
	cs.mu.Lock()
	safeContract, ok := cs.contracts[id]
	cs.mu.Unlock()
	if !ok {
		return nil, false
	}
	safeContract.mu.Lock()
	return safeContract, true
}

// Delete removes a contract from the set. The contract must have been
// previously acquired by Acquire. If the contract is not present in the set,
// Delete is a no-op.
func (cs *ContractSet) Delete(c *SafeContract) {
	cs.mu.Lock()
	safeContract, ok := cs.contracts[c.header.ID()]
	if !ok {
		cs.mu.Unlock()
		return
	}
	delete(cs.contracts, c.header.ID())
	cs.mu.Unlock()
	safeContract.mu.Unlock()
	// delete contract file
	os.Remove(filepath.Join(cs.dir, c.header.ID().String()+".contract"))
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

// Len returns the number of contracts in the set.
func (cs *ContractSet) Len() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.contracts)
}

// Return returns a locked contract to the set and unlocks it. The contract
// must have been previously acquired by Acquire. If the contract is not
// present in the set, Return panics.
func (cs *ContractSet) Return(c *SafeContract) {
	cs.mu.Lock()
	safeContract, ok := cs.contracts[c.header.ID()]
	if !ok {
		cs.mu.Unlock()
		build.Critical("no contract with that id")
	}
	cs.mu.Unlock()
	safeContract.mu.Unlock()
}

// View returns a copy of the contract with the specified ID. The contracts is
// not locked. Certain fields, including the MerkleRoots, are set to nil for
// safety reasons. If the contract is not present in the set, View
// returns false and a zero-valued RenterContract.
func (cs *ContractSet) View(id types.FileContractID) (modules.RenterContract, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	safeContract, ok := cs.contracts[id]
	if !ok {
		return modules.RenterContract{}, false
	}
	return safeContract.Metadata(), true
}

// ViewAll returns the metadata of each contract in the set. The contracts are
// not locked.
func (cs *ContractSet) ViewAll() []modules.RenterContract {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	contracts := make([]modules.RenterContract, 0, len(cs.contracts))
	for _, safeContract := range cs.contracts {
		contracts = append(contracts, safeContract.Metadata())
	}
	return contracts
}

func (cs *ContractSet) Close() error {
	for _, c := range cs.contracts {
		c.f.Close()
	}
	return cs.wal.Close()
}

// NewContractSet returns a ContractSet storing its contracts in the specified
// dir.
func NewContractSet(dir string) (*ContractSet, error) {
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	} else if stat, err := d.Stat(); err != nil {
		return nil, err
	} else if !stat.IsDir() {
		return nil, errors.New("not a directory")
	}
	defer d.Close()

	// Load the WAL. Any recovered updates will be applied after loading
	// contracts.
	updates, wal, err := writeaheadlog.New(filepath.Join(dir, "contractset.log"))
	if err != nil {
		return nil, err
	}

	cs := &ContractSet{
		contracts: make(map[types.FileContractID]*SafeContract),
		wal:       wal,
		dir:       dir,
	}

	// Load the contract files.
	dirNames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	for _, filename := range dirNames {
		if filepath.Ext(filename) != "contract" {
			continue
		}
		if err := cs.loadSafeContract(filename); err != nil {
			return nil, err
		}
	}

	// Apply any recovered updates.
	for _, update := range updates {
		switch update.Name {
		case updateNameSetHeader:
			var u updateSetHeader
			if err := encoding.Unmarshal(update.Instructions, &u); err != nil {
				return nil, err
			}
			if c, ok := cs.contracts[u.ID]; !ok {
				return nil, errors.New("no such contract")
			} else if err := c.applySetHeader(u.Header); err != nil {
				return nil, err
			} else if err := c.f.Sync(); err != nil {
				return nil, err
			}
		case updateNameSetRoot:
			var u updateSetRoot
			if err := encoding.Unmarshal(update.Instructions, &u); err != nil {
				return nil, err
			}
			if c, ok := cs.contracts[u.ID]; !ok {
				return nil, errors.New("no such contract")
			} else if err := c.applySetRoot(u.Root, u.Index); err != nil {
				return nil, err
			} else if err := c.f.Sync(); err != nil {
				return nil, err
			}
		}
	}

	return cs, nil
}
