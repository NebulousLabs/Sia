package proto

import (
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

	f   *os.File // TODO: use a dependency for this
	wal *writeaheadlog.WAL
	mu  sync.Mutex
}

func (c *SafeContract) Metadata() modules.RenterContract {
	c.headerMu.Lock()
	defer c.headerMu.Unlock()
	h := c.header
	return modules.RenterContract{
		ID:               h.ID(),
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
	}
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

func (c *SafeContract) recordUpload(txn types.Transaction, root crypto.Hash, storageCost, bandwidthCost types.Currency) error {
	// construct new header
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction = txn
	newHeader.StorageSpending = newHeader.StorageSpending.Add(storageCost)
	newHeader.UploadSpending = newHeader.UploadSpending.Add(bandwidthCost)

	rootIndex := len(c.merkleRoots)
	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
		c.makeUpdateSetRoot(root, rootIndex),
	})
	if err != nil {
		return err
	}
	if err := <-t.SignalSetupComplete(); err != nil {
		return err
	}

	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	if err := c.applySetRoot(root, rootIndex); err != nil {
		return err
	}
	if err := c.f.Sync(); err != nil {
		return err
	}
	return t.SignalUpdatesApplied()
}

func (c *SafeContract) recordDownload(txn types.Transaction, bandwidthCost types.Currency) error {
	// construct new header
	c.headerMu.Lock()
	newHeader := c.header
	c.headerMu.Unlock()
	newHeader.Transaction = txn
	newHeader.DownloadSpending = newHeader.DownloadSpending.Add(bandwidthCost)

	t, err := c.wal.NewTransaction([]writeaheadlog.Update{
		c.makeUpdateSetHeader(newHeader),
	})
	if err != nil {
		return err
	}
	if err := <-t.SignalSetupComplete(); err != nil {
		return err
	}
	if err := c.applySetHeader(newHeader); err != nil {
		return err
	}
	if err := c.f.Sync(); err != nil {
		return err
	}
	return t.SignalUpdatesApplied()
}

func (cs *ContractSet) managedInsertContract(h contractHeader, roots []crypto.Hash) (modules.RenterContract, error) {
	if err := h.validate(); err != nil {
		return modules.RenterContract{}, err
	}
	f, err := os.Create(filepath.Join(cs.dir, h.ID().String()+".contract"))
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

func (cs *ContractSet) loadSafeContract(filename string) error {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
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
	// add to set
	cs.contracts[header.ID()] = &SafeContract{
		header:      header,
		merkleRoots: merkleRoots,
		f:           f,
		wal:         cs.wal,
	}
	return nil
}
