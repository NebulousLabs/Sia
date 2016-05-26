package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

// An Editor modifies a Contract by communicating with a host. It uses the
// contract revision protocol to send modification requests to the host.
// Editors are the means by which the renter uploads data to hosts.
type Editor interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the Merkle root of the data.
	Upload(data []byte) (root crypto.Hash, err error)

	// Delete removes a sector from the underlying contract.
	Delete(crypto.Hash) error

	// Modify overwrites a sector with new data. Because the Editor does not
	// have access to the original sector data, the new Merkle root must be
	// supplied by the caller.
	Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error

	// Address returns the address of the host.
	Address() modules.NetAddress

	// ContractID returns the FileContractID of the contract.
	ContractID() types.FileContractID

	// EndHeight returns the height at which the contract ends.
	EndHeight() types.BlockHeight

	// Close terminates the connection to the host.
	Close() error
}

// A hostEditor modifies a Contract by calling the revise RPC on a host. It
// implements the Editor interface. hostEditors are NOT thread-safe; calls to
// Upload must happen in serial.
type hostEditor struct {
	editor     *proto.Editor
	contract   modules.RenterContract
	contractor *Contractor
}

// Address returns the NetAddress of the host.
func (he *hostEditor) Address() modules.NetAddress { return he.contract.NetAddress }

// ContractID returns the ID of the contract being revised.
func (he *hostEditor) ContractID() types.FileContractID { return he.contract.ID }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (he *hostEditor) EndHeight() types.BlockHeight { return he.contract.FileContract.WindowStart }

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *hostEditor) Close() error { return he.editor.Close() }

// Upload negotiates a revision that adds a sector to a file contract.
func (he *hostEditor) Upload(data []byte) (crypto.Hash, error) {
	oldUploadSpending := he.editor.UploadSpending
	oldStorageSpending := he.editor.StorageSpending
	contract, sectorRoot, err := he.editor.Upload(data)
	if err != nil {
		return crypto.Hash{}, err
	}
	uploadDelta := he.editor.UploadSpending.Sub(oldUploadSpending)
	storageDelta := he.editor.StorageSpending.Sub(oldStorageSpending)

	he.contractor.mu.Lock()
	he.contractor.financialMetrics.UploadSpending = he.contractor.financialMetrics.UploadSpending.Add(uploadDelta)
	he.contractor.financialMetrics.StorageSpending = he.contractor.financialMetrics.StorageSpending.Add(storageDelta)
	he.contractor.contracts[contract.ID] = contract
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	he.contract = contract

	return sectorRoot, nil
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *hostEditor) Delete(root crypto.Hash) error {
	contract, err := he.editor.Delete(root)
	if err != nil {
		return err
	}

	he.contractor.mu.Lock()
	he.contractor.contracts[contract.ID] = contract
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	he.contract = contract

	return nil
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *hostEditor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error {
	oldUploadSpending := he.editor.UploadSpending
	contract, err := he.editor.Modify(oldRoot, newRoot, offset, newData)
	if err != nil {
		return err
	}
	uploadDelta := he.editor.UploadSpending.Sub(oldUploadSpending)

	he.contractor.mu.Lock()
	he.contractor.financialMetrics.UploadSpending = he.contractor.financialMetrics.UploadSpending.Add(uploadDelta)
	he.contractor.contracts[contract.ID] = contract
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	he.contract = contract

	return nil
}

// Editor initiates the contract revision process with a host, and returns
// an Editor.
func (c *Contractor) Editor(contract modules.RenterContract) (Editor, error) {
	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()
	if height > contract.FileContract.WindowStart {
		return nil, errors.New("contract has already ended")
	}
	host, ok := c.hdb.Host(contract.NetAddress)
	if !ok {
		return nil, errors.New("no record of that host")
	}
	if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return nil, errTooExpensive
	}

	// create editor
	e, err := proto.NewEditor(host, contract, height)
	if err != nil {
		return nil, err
	}

	return &hostEditor{
		editor:     e,
		contractor: c,
	}, nil
}
