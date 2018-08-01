package proto

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/ratelimit"

	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/writeaheadlog"
)

// A ContractSet provides safe concurrent access to a set of contracts. Its
// purpose is to serialize modifications to individual contracts, as well as
// to provide operations on the set as a whole.
type ContractSet struct {
	contracts map[types.FileContractID]*SafeContract
	pubKeys   map[string]types.FileContractID
	deps      modules.Dependencies
	dir       string
	mu        sync.Mutex
	rl        *ratelimit.RateLimit
	wal       *writeaheadlog.WAL
}

// Acquire looks up the contract for the specified host key and locks it before
// returning it. If the contract is not present in the set, Acquire returns
// false and a zero-valued RenterContract.
func (cs *ContractSet) Acquire(id types.FileContractID) (*SafeContract, bool) {
	cs.mu.Lock()
	safeContract, ok := cs.contracts[id]
	cs.mu.Unlock()
	if !ok {
		return nil, false
	}
	safeContract.mu.Lock()
	// We need to check if the contract is still in the map or if it has been
	// deleted in the meantime.
	cs.mu.Lock()
	_, ok = cs.contracts[id]
	cs.mu.Unlock()
	if !ok {
		safeContract.mu.Unlock()
		return nil, false
	}
	return safeContract, true
}

// Delete removes a contract from the set. The contract must have been
// previously acquired by Acquire. If the contract is not present in the set,
// Delete is a no-op.
func (cs *ContractSet) Delete(c *SafeContract) {
	cs.mu.Lock()
	_, ok := cs.contracts[c.header.ID()]
	if !ok {
		cs.mu.Unlock()
		build.Critical("Delete called on already deleted contract")
		return
	}
	delete(cs.contracts, c.header.ID())
	delete(cs.pubKeys, string(c.header.HostPublicKey().Key))
	cs.mu.Unlock()
	c.mu.Unlock()
	// delete contract file
	path := filepath.Join(cs.dir, c.header.ID().String()+contractExtension)
	err := errors.Compose(c.headerFile.Close(), os.Remove(path))
	if err != nil {
		build.Critical("Failed to delete SafeContract from disk:", err)
	}
}

// IDs returns the fcid of each contract with in the set. The contracts are not
// locked.
func (cs *ContractSet) IDs() []types.FileContractID {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	pks := make([]types.FileContractID, 0, len(cs.contracts))
	for fcid := range cs.contracts {
		pks = append(pks, fcid)
	}
	return pks
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
	_, ok := cs.contracts[c.header.ID()]
	if !ok {
		cs.mu.Unlock()
		build.Critical("no contract with that key")
	}
	cs.mu.Unlock()
	c.mu.Unlock()
}

// RateLimits sets the bandwidth limits for connections created by the
// contractSet.
func (cs *ContractSet) RateLimits() (readBPS int64, writeBPS int64, packetSize uint64) {
	return cs.rl.Limits()
}

// SetRateLimits sets the bandwidth limits for connections created by the
// contractSet.
func (cs *ContractSet) SetRateLimits(readBPS int64, writeBPS int64, packetSize uint64) {
	cs.rl.SetLimits(readBPS, writeBPS, packetSize)
}

// View returns a copy of the contract with the specified host key. The
// contracts is not locked. Certain fields, including the MerkleRoots, are set
// to nil for safety reasons. If the contract is not present in the set, View
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

// Close closes all contracts in a contract set, this means rendering it unusable for I/O
func (cs *ContractSet) Close() error {
	for _, c := range cs.contracts {
		c.headerFile.Close()
	}
	_, err := cs.wal.CloseIncomplete()
	return err
}

// NewContractSet returns a ContractSet storing its contracts in the specified
// dir.
func NewContractSet(dir string, deps modules.Dependencies) (*ContractSet, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
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
	// COMPATv1.3.1RC2 Rename old wals to have the 'wal' extension if new file
	// doesn't exist.
	if err := v131RC2RenameWAL(dir); err != nil {
		return nil, err
	}
	walTxns, wal, err := writeaheadlog.New(filepath.Join(dir, "contractset.wal"))
	if err != nil {
		return nil, err
	}

	cs := &ContractSet{
		contracts: make(map[types.FileContractID]*SafeContract),
		pubKeys:   make(map[string]types.FileContractID),

		deps: deps,
		dir:  dir,
		wal:  wal,
	}
	// Set the initial rate limit to 'unlimited' bandwidth with 4kib packets.
	cs.rl = ratelimit.NewRateLimit(0, 0, 0)

	// Load the contract files.
	dirNames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	for _, filename := range dirNames {
		if filepath.Ext(filename) != contractExtension {
			continue
		}
		path := filepath.Join(dir, filename)
		if err := cs.loadSafeContract(path, walTxns); err != nil {
			return nil, err
		}
	}

	return cs, nil
}

// v131RC2RenameWAL renames an existing old wal file from contractset.log to
// contractset.wal
func v131RC2RenameWAL(dir string) error {
	oldPath := filepath.Join(dir, "contractset.log")
	newPath := filepath.Join(dir, "contractset.wal")
	_, errOld := os.Stat(oldPath)
	_, errNew := os.Stat(newPath)
	if !os.IsNotExist(errOld) && os.IsNotExist(errNew) {
		return build.ExtendErr("failed to rename contractset.log to contractset.wal",
			os.Rename(oldPath, newPath))
	}
	return nil
}
