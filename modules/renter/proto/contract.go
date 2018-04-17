package proto

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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

	// merkleRoots are the Merkle roots of each sector stored on the host that
	// relate to this contract.
	merkleRoots []crypto.Hash

	// unappliedTxns are the transactions that were written to the WAL but not
	// applied to the contract file.
	unappliedTxns []*writeaheadlog.Transaction

	f   *os.File // TODO: use a dependency for this
	wal *writeaheadlog.WAL
	mu  sync.Mutex
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
	if err := c.f.Sync(); err != nil {
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
	if _, err := c.f.WriteAt(headerBytes, 0); err != nil {
		return err
	}
	c.headerMu.Lock()
	c.header = h
	c.headerMu.Unlock()
	return nil
}

func (c *SafeContract) applySetRoot(root crypto.Hash, index int) error {
	rootOffset := contractHeaderSize + crypto.HashSize*int64(index)
	if _, err := c.f.WriteAt(root[:], rootOffset); err != nil {
		return err
	}
	if len(c.merkleRoots) <= index {
		c.merkleRoots = append(c.merkleRoots, make([]crypto.Hash, 1+index-len(c.merkleRoots))...)
	}
	c.merkleRoots[index] = root
	return nil
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

	rootIndex := len(c.merkleRoots)
	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
		c.makeUpdateSetRoot(root, rootIndex),
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

	rootIndex := len(c.merkleRoots)
	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	if err := c.applySetRoot(root, rootIndex); err != nil {
		return err
	}
	if err := c.f.Sync(); err != nil {
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
	if err := c.f.Sync(); err != nil {
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
		if err := c.f.Sync(); err != nil {
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
	// preallocate space for header + roots
	if err := f.Truncate(contractHeaderSize + crypto.HashSize*int64(len(roots))); err != nil {
		return modules.RenterContract{}, err
	}
	// write header
	if _, err := f.WriteAt(encoding.Marshal(h), 0); err != nil {
		return modules.RenterContract{}, err
	}
	// write roots
	for i, root := range roots {
		if _, err := f.WriteAt(root[:], contractHeaderSize+crypto.HashSize*int64(i)); err != nil {
			return modules.RenterContract{}, err
		}
	}
	if err := f.Sync(); err != nil {
		return modules.RenterContract{}, err
	}
	sc := &SafeContract{
		header:      h,
		merkleRoots: roots,
		f:           f,
		wal:         cs.wal,
	}
	cs.mu.Lock()
	cs.contracts[h.ID()] = sc
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
	// read header
	var header contractHeader
	if err := encoding.NewDecoder(f).Decode(&header); err != nil {
		return err
	} else if err := header.validate(); err != nil {
		return err
	}
	// read merkleRoots
	var merkleRoots []crypto.Hash
	if _, err := f.Seek(contractHeaderSize, io.SeekStart); err != nil {
		return err
	}
	for {
		var root crypto.Hash
		if _, err := io.ReadFull(f, root[:]); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		merkleRoots = append(merkleRoots, root)
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
	cs.contracts[header.ID()] = &SafeContract{
		header:        header,
		merkleRoots:   merkleRoots,
		unappliedTxns: unappliedTxns,
		f:             f,
		wal:           cs.wal,
	}
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
		if len(cr.MerkleRoots) == len(sc.merkleRoots)+1 {
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
