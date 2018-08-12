package contractor

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

var errInvalidEditor = errors.New("editor has been invalidated because its contract is being renewed")

// the contractor will cap host's MaxCollateral setting to this value
var maxUploadCollateral = types.SiacoinPrecision.Mul64(1e3).Div(modules.BlockBytesPerMonthTerabyte) // 1k SC / TB / Month

// An Editor modifies a Contract by communicating with a host. It uses the
// contract revision protocol to send modification requests to the host.
// Editors are the means by which the renter uploads data to hosts.
type Editor interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the Merkle root of the data.
	Upload(data []byte) (root crypto.Hash, err error)

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
// implements the Editor interface. hostEditors are safe for use by
// multiple goroutines.
type hostEditor struct {
	clients    int // safe to Close when 0
	contractor *Contractor
	editor     *proto.Editor
	endHeight  types.BlockHeight
	id         types.FileContractID
	invalid    bool // true if invalidate has been called
	netAddress modules.NetAddress

	mu sync.Mutex
}

// invalidate sets the invalid flag and closes the underlying proto.Editor.
// Once invalidate returns, the hostEditor is guaranteed to not further revise
// its contract. This is used during contract renewal to prevent an Editor
// from revising a contract mid-renewal.
func (he *hostEditor) invalidate() {
	he.mu.Lock()
	defer he.mu.Unlock()
	if !he.invalid {
		he.editor.Close()
		he.invalid = true
	}
	he.contractor.mu.Lock()
	delete(he.contractor.editors, he.id)
	delete(he.contractor.revising, he.id)
	he.contractor.mu.Unlock()
}

// Address returns the NetAddress of the host.
func (he *hostEditor) Address() modules.NetAddress { return he.netAddress }

// ContractID returns the id of the contract being revised.
func (he *hostEditor) ContractID() types.FileContractID { return he.id }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (he *hostEditor) EndHeight() types.BlockHeight { return he.endHeight }

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *hostEditor) Close() error {
	he.mu.Lock()
	defer he.mu.Unlock()
	he.clients--
	// Close is a no-op if invalidate has been called, or if there are other
	// clients still using the hostEditor.
	if he.invalid || he.clients > 0 {
		return nil
	}
	he.invalid = true
	he.contractor.mu.Lock()
	delete(he.contractor.editors, he.id)
	delete(he.contractor.revising, he.id)
	he.contractor.mu.Unlock()
	return he.editor.Close()
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *hostEditor) Upload(data []byte) (_ crypto.Hash, err error) {
	he.mu.Lock()
	defer he.mu.Unlock()
	if he.invalid {
		return crypto.Hash{}, errInvalidEditor
	}

	// Perform the upload.
	_, sectorRoot, err := he.editor.Upload(data)
	if err != nil {
		return crypto.Hash{}, err
	}
	return sectorRoot, nil
}

// Editor returns a Editor object that can be used to upload, modify, and
// delete sectors on a host.
func (c *Contractor) Editor(pk types.SiaPublicKey, cancel <-chan struct{}) (_ Editor, err error) {
	c.mu.RLock()
	id, gotID := c.pubKeysToContractID[string(pk.Key)]
	cachedEditor, haveEditor := c.editors[id]
	height := c.blockHeight
	renewing := c.renewing[id]
	c.mu.RUnlock()
	if !gotID {
		return nil, errors.New("failed to get filecontract id from key")
	}
	if renewing {
		// Cannot use the editor if the contract is being renewed.
		return nil, errors.New("currently renewing that contract")
	} else if haveEditor {
		// This editor already exists. Mark that there are now two routines
		// using the editor, and then return the editor that already exists.
		cachedEditor.mu.Lock()
		cachedEditor.clients++
		cachedEditor.mu.Unlock()
		return cachedEditor, nil
	}

	// Check that the contract and host are both available, and run some brief
	// sanity checks to see that the host is not swindling us.
	contract, haveContract := c.staticContracts.View(id)
	if !haveContract {
		return nil, errors.New("no record of that contract")
	}
	host, haveHost := c.hdb.Host(contract.HostPublicKey)
	if height > contract.EndHeight {
		return nil, errors.New("contract has already ended")
	} else if !haveHost {
		return nil, errors.New("no record of that host")
	} else if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return nil, errTooExpensive
	} else if host.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
		return nil, errTooExpensive
	}

	// Acquire the revising lock.
	c.mu.Lock()
	alreadyRevising := c.revising[contract.ID]
	if alreadyRevising {
		c.mu.Unlock()
		return nil, errors.New("already revising that contract")
	}
	c.revising[contract.ID] = true
	c.mu.Unlock()
	// Release the revising lock early in the event of an error.
	defer func() {
		if err != nil {
			c.mu.Lock()
			delete(c.revising, contract.ID)
			c.mu.Unlock()
		}
	}()

	// Create the editor.
	e, err := c.staticContracts.NewEditor(host, contract.ID, height, c.hdb, cancel)
	if err != nil {
		return nil, err
	}

	// cache editor
	he := &hostEditor{
		clients:    1,
		contractor: c,
		editor:     e,
		endHeight:  contract.EndHeight,
		id:         id,
		netAddress: host.NetAddress,
	}
	c.mu.Lock()
	c.editors[contract.ID] = he
	c.mu.Unlock()

	return he, nil
}
