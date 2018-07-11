package proto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/writeaheadlog"
)

const (
	// contractHeaderSize is the maximum amount of space that the non-Merkle-root
	// portion of a contract can consume.
	contractHeaderSize = writeaheadlog.MaxPayloadSize // TODO: test this

	updateNameSetHeader = "setHeader"
	updateNameSetRoot   = "setRoot"
)

type updateSetHeader struct {
	ID     types.FileContractID
	Header contractHeader
}

// v132UpdateHeader was introduced due to backwards compatibility reasons after
// changing the format of the contractHeader. It contains the legacy
// v132ContractHeader.
type v132UpdateSetHeader struct {
	ID     types.FileContractID
	Header v132ContractHeader
}

type updateSetRoot struct {
	ID    types.FileContractID
	Root  crypto.Hash
	Index int
}

type contractHeader struct {
	// transaction is the signed transaction containing the most recent
	// revision of the file contract.
	Transaction types.Transaction

	// secretKey is the key used by the renter to sign the file contract
	// transaction.
	SecretKey crypto.SecretKey

	// Same as modules.RenterContract.
	StartHeight      types.BlockHeight
	DownloadSpending types.Currency
	StorageSpending  types.Currency
	UploadSpending   types.Currency
	TotalCost        types.Currency
	ContractFee      types.Currency
	TxnFee           types.Currency
	SiafundFee       types.Currency
	Utility          modules.ContractUtility
}

// v132ContractHeader is a contractHeader without the Utility field. This field
// was added after v132 to be able to persist contract utilities.
type v132ContractHeader struct {
	// transaction is the signed transaction containing the most recent
	// revision of the file contract.
	Transaction types.Transaction

	// secretKey is the key used by the renter to sign the file contract
	// transaction.
	SecretKey crypto.SecretKey

	// Same as modules.RenterContract.
	StartHeight      types.BlockHeight
	DownloadSpending types.Currency
	StorageSpending  types.Currency
	UploadSpending   types.Currency
	TotalCost        types.Currency
	ContractFee      types.Currency
	TxnFee           types.Currency
	SiafundFee       types.Currency
}

// validate returns an error if the contractHeader is invalid.
func (h *contractHeader) validate() error {
	if len(h.Transaction.FileContractRevisions) > 0 &&
		len(h.Transaction.FileContractRevisions[0].NewValidProofOutputs) > 0 &&
		len(h.Transaction.FileContractRevisions[0].UnlockConditions.PublicKeys) == 2 {
		return nil
	}
	return errors.New("invalid contract")
}

func (h *contractHeader) copyTransaction() (txn types.Transaction) {
	encoding.Unmarshal(encoding.Marshal(h.Transaction), &txn)
	return
}

func (h *contractHeader) LastRevision() types.FileContractRevision {
	return h.Transaction.FileContractRevisions[0]
}

func (h *contractHeader) ID() types.FileContractID {
	return h.LastRevision().ParentID
}

func (h *contractHeader) HostPublicKey() types.SiaPublicKey {
	return h.LastRevision().UnlockConditions.PublicKeys[1]
}

func (h *contractHeader) RenterFunds() types.Currency {
	return h.LastRevision().NewValidProofOutputs[0].Value
}

func (h *contractHeader) EndHeight() types.BlockHeight {
	return h.LastRevision().NewWindowStart
}

// A SafeContract contains the most recent revision transaction negotiated
// with a host, and the secret key used to sign it.
type SafeContract struct {
	headerMu sync.Mutex
	header   contractHeader

	// merkleRoots are the sector roots covered by this contract.
	merkleRoots *merkleRoots

	// unappliedTxns are the transactions that were written to the WAL but not
	// applied to the contract file.
	unappliedTxns []*writeaheadlog.Transaction

	headerFile *fileSection
	wal        *writeaheadlog.WAL
	mu         sync.Mutex
}

// Metadata returns the metadata of a renter contract
func (c *SafeContract) Metadata() modules.RenterContract {
	c.headerMu.Lock()
	defer c.headerMu.Unlock()
	h := c.header
	return modules.RenterContract{
		ID:               h.ID(),
		Transaction:      h.copyTransaction(),
		HostPublicKey:    h.HostPublicKey(),
		StartHeight:      h.StartHeight,
		EndHeight:        h.EndHeight(),
		RenterFunds:      h.RenterFunds(),
		DownloadSpending: h.DownloadSpending,
		StorageSpending:  h.StorageSpending,
		UploadSpending:   h.UploadSpending,
		TotalCost:        h.TotalCost,
		ContractFee:      h.ContractFee,
		TxnFee:           h.TxnFee,
		SiafundFee:       h.SiafundFee,
		Utility:          h.Utility,
	}
}

// UpdateUtility updates the utility field of a contract.
func (c *SafeContract) UpdateUtility(utility modules.ContractUtility) error {
	// Get current header
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()

	// Construct new header
	newHeader.Utility = utility

	// Record the intent to change the header in the wal.
	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
	})
	if err != nil {
		return err
	}
	// Signal that the setup is completed.
	if err := <-t.SignalSetupComplete(); err != nil {
		return err
	}
	// Apply the change.
	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	// Sync the change to disk.
	if err := c.headerFile.Sync(); err != nil {
		return err
	}
	// Signal that the update has been applied.
	if err := t.SignalUpdatesApplied(); err != nil {
		return err
	}
	return nil
}

// Utility returns the contract utility for the contract.
func (c *SafeContract) Utility() modules.ContractUtility {
	c.headerMu.Lock()
	defer c.headerMu.Unlock()
	return c.header.Utility
}

func (c *SafeContract) makeUpdateSetHeader(h contractHeader) writeaheadlog.Update {
	c.headerMu.Lock()
	id := c.header.ID()
	c.headerMu.Unlock()
	return writeaheadlog.Update{
		Name: updateNameSetHeader,
		Instructions: encoding.Marshal(updateSetHeader{
			ID:     id,
			Header: h,
		}),
	}
}

func (c *SafeContract) makeUpdateSetRoot(root crypto.Hash, index int) writeaheadlog.Update {
	c.headerMu.Lock()
	id := c.header.ID()
	c.headerMu.Unlock()
	return writeaheadlog.Update{
		Name: updateNameSetRoot,
		Instructions: encoding.Marshal(updateSetRoot{
			ID:    id,
			Root:  root,
			Index: index,
		}),
	}
}

func (c *SafeContract) applySetHeader(h contractHeader) error {
	headerBytes := make([]byte, contractHeaderSize)
	copy(headerBytes, encoding.Marshal(h))
	if _, err := c.headerFile.WriteAt(headerBytes, 0); err != nil {
		return err
	}
	c.headerMu.Lock()
	c.header = h
	c.headerMu.Unlock()
	return nil
}

func (c *SafeContract) applySetRoot(root crypto.Hash, index int) error {
	return c.merkleRoots.insert(index, root)
}

func (c *SafeContract) recordUploadIntent(rev types.FileContractRevision, root crypto.Hash, storageCost, bandwidthCost types.Currency) (*writeaheadlog.Transaction, error) {
	// construct new header
	// NOTE: this header will not include the host signature
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction.FileContractRevisions = []types.FileContractRevision{rev}
	newHeader.StorageSpending = newHeader.StorageSpending.Add(storageCost)
	newHeader.UploadSpending = newHeader.UploadSpending.Add(bandwidthCost)

	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
		c.makeUpdateSetRoot(root, c.merkleRoots.len()),
	})
	if err != nil {
		return nil, err
	}
	if err := <-t.SignalSetupComplete(); err != nil {
		return nil, err
	}
	c.unappliedTxns = append(c.unappliedTxns, t)
	return t, nil
}

func (c *SafeContract) commitUpload(t *writeaheadlog.Transaction, signedTxn types.Transaction, root crypto.Hash, storageCost, bandwidthCost types.Currency) error {
	// construct new header
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction = signedTxn
	newHeader.StorageSpending = newHeader.StorageSpending.Add(storageCost)
	newHeader.UploadSpending = newHeader.UploadSpending.Add(bandwidthCost)

	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	if err := c.applySetRoot(root, c.merkleRoots.len()); err != nil {
		return err
	}
	if err := c.headerFile.Sync(); err != nil {
		return err
	}
	if err := t.SignalUpdatesApplied(); err != nil {
		return err
	}
	c.unappliedTxns = nil
	return nil
}

func (c *SafeContract) recordDownloadIntent(rev types.FileContractRevision, bandwidthCost types.Currency) (*writeaheadlog.Transaction, error) {
	// construct new header
	// NOTE: this header will not include the host signature
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction.FileContractRevisions = []types.FileContractRevision{rev}
	newHeader.DownloadSpending = newHeader.DownloadSpending.Add(bandwidthCost)

	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
	})
	if err != nil {
		return nil, err
	}
	if err := <-t.SignalSetupComplete(); err != nil {
		return nil, err
	}
	c.unappliedTxns = append(c.unappliedTxns, t)
	return t, nil
}

func (c *SafeContract) commitDownload(t *writeaheadlog.Transaction, signedTxn types.Transaction, bandwidthCost types.Currency) error {
	// construct new header
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction = signedTxn
	newHeader.DownloadSpending = newHeader.DownloadSpending.Add(bandwidthCost)

	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	if err := c.headerFile.Sync(); err != nil {
		return err
	}
	if err := t.SignalUpdatesApplied(); err != nil {
		return err
	}
	c.unappliedTxns = nil
	return nil
}

// commitTxns commits the unapplied transactions to the contract file and marks
// the transactions as applied.
func (c *SafeContract) commitTxns() error {
	for _, t := range c.unappliedTxns {
		for _, update := range t.Updates {
			switch update.Name {
			case updateNameSetHeader:
				var u updateSetHeader
				if err := unmarshalHeader(update.Instructions, &u); err != nil {
					return err
				}
				if err := c.applySetHeader(u.Header); err != nil {
					return err
				}
			case updateNameSetRoot:
				var u updateSetRoot
				if err := encoding.Unmarshal(update.Instructions, &u); err != nil {
					return err
				}
				if err := c.applySetRoot(u.Root, u.Index); err != nil {
					return err
				}
			}
		}
		if err := c.headerFile.Sync(); err != nil {
			return err
		}
		if err := t.SignalUpdatesApplied(); err != nil {
			return err
		}
	}
	c.unappliedTxns = nil
	return nil
}

// unappliedHeader returns the most recent header contained within the unapplied
// transactions relevant to the contract.
func (c *SafeContract) unappliedHeader() (h contractHeader) {
	for _, t := range c.unappliedTxns {
		for _, update := range t.Updates {
			if update.Name == updateNameSetHeader {
				var u updateSetHeader
				if err := unmarshalHeader(update.Instructions, &u); err != nil {
					continue
				}
				h = u.Header
			}
		}
	}
	return
}

func (cs *ContractSet) managedInsertContract(h contractHeader, roots []crypto.Hash) (modules.RenterContract, error) {
	if err := h.validate(); err != nil {
		return modules.RenterContract{}, err
	}
	f, err := os.Create(filepath.Join(cs.dir, h.ID().String()+contractExtension))
	if err != nil {
		return modules.RenterContract{}, err
	}
	// create fileSections
	headerSection := newFileSection(f, 0, contractHeaderSize)
	rootsSection := newFileSection(f, contractHeaderSize, -1)
	// write header
	if _, err := headerSection.WriteAt(encoding.Marshal(h), 0); err != nil {
		return modules.RenterContract{}, err
	}
	// write roots
	merkleRoots := newMerkleRoots(rootsSection)
	for _, root := range roots {
		if err := merkleRoots.push(root); err != nil {
			return modules.RenterContract{}, err
		}
	}
	if err := f.Sync(); err != nil {
		return modules.RenterContract{}, err
	}
	sc := &SafeContract{
		header:      h,
		merkleRoots: merkleRoots,
		headerFile:  headerSection,
		wal:         cs.wal,
	}
	cs.mu.Lock()
	cs.contracts[sc.header.ID()] = sc
	cs.pubKeys[string(h.HostPublicKey().Key)] = sc.header.ID()
	cs.mu.Unlock()
	return sc.Metadata(), nil
}

// loadSafeContract loads a contract from disk and adds it to the contractset
// if it is valid.
func (cs *ContractSet) loadSafeContract(filename string, walTxns []*writeaheadlog.Transaction) error {
	f, err := os.OpenFile(filename, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	headerSection := newFileSection(f, 0, contractHeaderSize)
	rootsSection := newFileSection(f, contractHeaderSize, remainingFile)

	// read header
	var header contractHeader
	if err := encoding.NewDecoder(f).Decode(&header); err != nil {
		return err
	} else if err := header.validate(); err != nil {
		return err
	}

	// read merkleRoots
	merkleRoots, err := loadExistingMerkleRoots(rootsSection)
	if err != nil {
		return err
	}
	// add relevant unapplied transactions
	var unappliedTxns []*writeaheadlog.Transaction
	for _, t := range walTxns {
		// NOTE: we assume here that if any of the updates apply to the
		// contract, the whole transaction applies to the contract.
		if len(t.Updates) == 0 {
			continue
		}
		var id types.FileContractID
		switch update := t.Updates[0]; update.Name {
		case updateNameSetHeader:
			var u updateSetHeader
			if err := unmarshalHeader(update.Instructions, &u); err != nil {
				return err
			}
			id = u.ID
		case updateNameSetRoot:
			var u updateSetRoot
			if err := encoding.Unmarshal(update.Instructions, &u); err != nil {
				return err
			}
			id = u.ID
		}
		if id == header.ID() {
			unappliedTxns = append(unappliedTxns, t)
		}
	}
	// add to set
	sc := &SafeContract{
		header:        header,
		merkleRoots:   merkleRoots,
		unappliedTxns: unappliedTxns,
		headerFile:    headerSection,
		wal:           cs.wal,
	}
	cs.contracts[sc.header.ID()] = sc
	cs.pubKeys[string(header.HostPublicKey().Key)] = sc.header.ID()
	return nil
}

// ConvertV130Contract creates a contract file for a v130 contract.
func (cs *ContractSet) ConvertV130Contract(c V130Contract, cr V130CachedRevision) error {
	m, err := cs.managedInsertContract(contractHeader{
		Transaction:      c.LastRevisionTxn,
		SecretKey:        c.SecretKey,
		StartHeight:      c.StartHeight,
		DownloadSpending: c.DownloadSpending,
		StorageSpending:  c.StorageSpending,
		UploadSpending:   c.UploadSpending,
		TotalCost:        c.TotalCost,
		ContractFee:      c.ContractFee,
		TxnFee:           c.TxnFee,
		SiafundFee:       c.SiafundFee,
	}, c.MerkleRoots)
	if err != nil {
		return err
	}
	// if there is a cached revision, store it as an unapplied WAL transaction
	if cr.Revision.NewRevisionNumber != 0 {
		sc, ok := cs.Acquire(m.ID)
		if !ok {
			return errors.New("contract set is missing contract that was just added")
		}
		defer cs.Return(sc)
		if len(cr.MerkleRoots) == sc.merkleRoots.len()+1 {
			root := cr.MerkleRoots[len(cr.MerkleRoots)-1]
			_, err = sc.recordUploadIntent(cr.Revision, root, types.ZeroCurrency, types.ZeroCurrency)
		} else {
			_, err = sc.recordDownloadIntent(cr.Revision, types.ZeroCurrency)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// A V130Contract specifies the v130 contract format.
type V130Contract struct {
	HostPublicKey    types.SiaPublicKey         `json:"hostpublickey"`
	ID               types.FileContractID       `json:"id"`
	LastRevision     types.FileContractRevision `json:"lastrevision"`
	LastRevisionTxn  types.Transaction          `json:"lastrevisiontxn"`
	MerkleRoots      MerkleRootSet              `json:"merkleroots"`
	SecretKey        crypto.SecretKey           `json:"secretkey"`
	StartHeight      types.BlockHeight          `json:"startheight"`
	DownloadSpending types.Currency             `json:"downloadspending"`
	StorageSpending  types.Currency             `json:"storagespending"`
	UploadSpending   types.Currency             `json:"uploadspending"`
	TotalCost        types.Currency             `json:"totalcost"`
	ContractFee      types.Currency             `json:"contractfee"`
	TxnFee           types.Currency             `json:"txnfee"`
	SiafundFee       types.Currency             `json:"siafundfee"`
}

// EndHeight returns the height at which the host is no longer obligated to
// store contract data.
func (c *V130Contract) EndHeight() types.BlockHeight {
	return c.LastRevision.NewWindowStart
}

// RenterFunds returns the funds remaining in the contract's Renter payout as
// of the most recent revision.
func (c *V130Contract) RenterFunds() types.Currency {
	if len(c.LastRevision.NewValidProofOutputs) < 2 {
		return types.ZeroCurrency
	}
	return c.LastRevision.NewValidProofOutputs[0].Value
}

// A V130CachedRevision contains changes that would be applied to a
// RenterContract if a contract revision succeeded.
type V130CachedRevision struct {
	Revision    types.FileContractRevision `json:"revision"`
	MerkleRoots modules.MerkleRootSet      `json:"merkleroots"`
}

// MerkleRootSet is a set of Merkle roots, and gets encoded more efficiently.
type MerkleRootSet []crypto.Hash

// MarshalJSON defines a JSON encoding for a MerkleRootSet.
func (mrs MerkleRootSet) MarshalJSON() ([]byte, error) {
	// Copy the whole array into a giant byte slice and then encode that.
	fullBytes := make([]byte, crypto.HashSize*len(mrs))
	for i := range mrs {
		copy(fullBytes[i*crypto.HashSize:(i+1)*crypto.HashSize], mrs[i][:])
	}
	return json.Marshal(fullBytes)
}

// UnmarshalJSON attempts to decode a MerkleRootSet, falling back on the legacy
// decoding of a []crypto.Hash if that fails.
func (mrs *MerkleRootSet) UnmarshalJSON(b []byte) error {
	// Decode the giant byte slice, and then split it into separate arrays.
	var fullBytes []byte
	err := json.Unmarshal(b, &fullBytes)
	if err != nil {
		// Encoding the byte slice has failed, try decoding it as a []crypto.Hash.
		var hashes []crypto.Hash
		err := json.Unmarshal(b, &hashes)
		if err != nil {
			return err
		}
		*mrs = MerkleRootSet(hashes)
		return nil
	}

	umrs := make(MerkleRootSet, len(fullBytes)/32)
	for i := range umrs {
		copy(umrs[i][:], fullBytes[i*crypto.HashSize:(i+1)*crypto.HashSize])
	}
	*mrs = umrs
	return nil
}

func unmarshalHeader(b []byte, u *updateSetHeader) error {
	// Try unmarshaling the header.
	if err := encoding.Unmarshal(b, u); err != nil {
		// COMPATv132 try unmarshaling the header the old way.
		var oldHeader v132UpdateSetHeader
		if err2 := encoding.Unmarshal(b, &oldHeader); err2 != nil {
			// If unmarshaling the header the old way also doesn't work we
			// return the original error.
			return err
		}
		// If unmarshaling it the old way was successful we convert it to a new
		// header.
		u.Header = contractHeader{
			Transaction:      oldHeader.Header.Transaction,
			SecretKey:        oldHeader.Header.SecretKey,
			StartHeight:      oldHeader.Header.StartHeight,
			DownloadSpending: oldHeader.Header.DownloadSpending,
			StorageSpending:  oldHeader.Header.StorageSpending,
			UploadSpending:   oldHeader.Header.UploadSpending,
			TotalCost:        oldHeader.Header.TotalCost,
			ContractFee:      oldHeader.Header.ContractFee,
			TxnFee:           oldHeader.Header.TxnFee,
			SiafundFee:       oldHeader.Header.SiafundFee,
		}
	}
	return nil
}
