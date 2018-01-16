package contractor

// The contractor achieves efficient persistence using a JSON transaction
// journal. It enables efficient ACID transactions on JSON objects.
//
// The journal represents a single JSON object, containing all of the
// contractor's persisted data. The object is serialized as an "initial
// object" followed by a series of update sets, one per line. Each update
// specifies a modification.
//
// During operation, the object is first loaded by reading the file and
// applying each update to the initial object. It is subsequently modified by
// appending update sets to the file, one per line. At any time, a
// "checkpoint" may be created, which clears the journal and starts over with
// a new initial object. This allows for compaction of the journal file.
//
// In the event of power failure or other serious disruption, the most recent
// update set may be only partially written. Partially written update sets are
// simply ignored when reading the journal.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var journalMeta = persist.Metadata{
	Header:  "Contractor Journal",
	Version: "1.1.1",
}

type journalPersist struct {
	Allowance       modules.Allowance                   `json:"allowance"`
	BlockHeight     types.BlockHeight                   `json:"blockheight"`
	CachedRevisions map[string]proto.V130CachedRevision `json:"cachedrevisions"`
	Contracts       map[string]proto.V130Contract       `json:"contracts"`
	CurrentPeriod   types.BlockHeight                   `json:"currentperiod"`
	LastChange      modules.ConsensusChangeID           `json:"lastchange"`
	OldContracts    []proto.V130Contract                `json:"oldcontracts"`
	RenewedIDs      map[string]string                   `json:"renewedids"`
}

// A journal is a log of updates to a JSON object.
type journal struct {
	f        *os.File
	filename string
}

// update applies the updateSet atomically to j. It syncs the underlying file
// before returning.
func (j *journal) update(us updateSet) error {
	if err := json.NewEncoder(j.f).Encode(us); err != nil {
		return err
	}
	return j.f.Sync()
}

// Close closes the underlying file.
func (j *journal) Close() error {
	return j.f.Close()
}

// openJournal opens the supplied journal and decodes the reconstructed
// journalPersist into data.
func openJournal(filename string, data *journalPersist) (*journal, error) {
	// Open file handle for reading and writing.
	f, err := os.OpenFile(filename, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	// Decode the metadata.
	dec := json.NewDecoder(f)
	var meta persist.Metadata
	if err = dec.Decode(&meta); err != nil {
		return nil, err
	} else if meta.Header != journalMeta.Header {
		return nil, fmt.Errorf("expected header %q, got %q", journalMeta.Header, meta.Header)
	} else if meta.Version != journalMeta.Version {
		return nil, fmt.Errorf("journal version (%s) is incompatible with the current version (%s)", meta.Version, journalMeta.Version)
	}

	// Decode the initial object.
	if err = dec.Decode(data); err != nil {
		return nil, err
	}

	// Make sure all maps are properly initialized.
	if data.CachedRevisions == nil {
		data.CachedRevisions = map[string]proto.V130CachedRevision{}
	}
	if data.Contracts == nil {
		data.Contracts = map[string]proto.V130Contract{}
	}
	if data.RenewedIDs == nil {
		data.RenewedIDs = map[string]string{}
	}

	// Decode each set of updates and apply them to data.
	for {
		var set updateSet
		if err = dec.Decode(&set); err == io.EOF || err == io.ErrUnexpectedEOF {
			// unexpected EOF means the last update was corrupted; skip it
			break
		} else if err != nil {
			// skip corrupted update sets
			continue
		}
		for _, u := range set {
			u.apply(data)
		}
	}

	return &journal{
		f:        f,
		filename: filename,
	}, nil
}

type journalUpdate interface {
	apply(*journalPersist)
}

type marshaledUpdate struct {
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Checksum crypto.Hash     `json:"checksum"`
}

type updateSet []journalUpdate

// UnmarshalJSON unmarshals an array of marshaledUpdates as a set of
// journalUpdates.
func (set *updateSet) UnmarshalJSON(b []byte) error {
	var marshaledSet []marshaledUpdate
	if err := json.Unmarshal(b, &marshaledSet); err != nil {
		return err
	}
	for _, u := range marshaledSet {
		if crypto.HashBytes(u.Data) != u.Checksum {
			return errors.New("bad checksum")
		}
		var err error
		switch u.Type {
		case "uploadRevision":
			var ur updateUploadRevision
			err = json.Unmarshal(u.Data, &ur)
			*set = append(*set, ur)
		case "downloadRevision":
			var dr updateDownloadRevision
			err = json.Unmarshal(u.Data, &dr)
			*set = append(*set, dr)
		case "cachedUploadRevision":
			var cur updateCachedUploadRevision
			err = json.Unmarshal(u.Data, &cur)
			*set = append(*set, cur)
		case "cachedDownloadRevision":
			var cdr updateCachedDownloadRevision
			err = json.Unmarshal(u.Data, &cdr)
			*set = append(*set, cdr)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// updateUploadRevision is a journalUpdate that records the new data
// associated with uploading a sector to a host.
type updateUploadRevision struct {
	NewRevisionTxn     types.Transaction `json:"newrevisiontxn"`
	NewSectorRoot      crypto.Hash       `json:"newsectorroot"`
	NewSectorIndex     int               `json:"newsectorindex"`
	NewUploadSpending  types.Currency    `json:"newuploadspending"`
	NewStorageSpending types.Currency    `json:"newstoragespending"`
}

// apply sets the LastRevision, LastRevisionTxn, UploadSpending, and
// DownloadSpending fields of the contract being revised. It also adds the new
// Merkle root to the contract's Merkle root set.
func (u updateUploadRevision) apply(data *journalPersist) {
	if len(u.NewRevisionTxn.FileContractRevisions) == 0 {
		build.Critical("updateUploadRevision is missing its FileContractRevision")
		return
	}

	rev := u.NewRevisionTxn.FileContractRevisions[0]
	c := data.Contracts[rev.ParentID.String()]
	c.LastRevisionTxn = u.NewRevisionTxn

	if u.NewSectorIndex == len(c.MerkleRoots) {
		c.MerkleRoots = append(c.MerkleRoots, u.NewSectorRoot)
	} else if u.NewSectorIndex < len(c.MerkleRoots) {
		c.MerkleRoots[u.NewSectorIndex] = u.NewSectorRoot
	} else {
		// Shouldn't happen. TODO: Correctly handle error.
	}

	c.UploadSpending = u.NewUploadSpending
	c.StorageSpending = u.NewStorageSpending
	data.Contracts[rev.ParentID.String()] = c
}

// updateUploadRevision is a journalUpdate that records the new data
// associated with downloading a sector from a host.
type updateDownloadRevision struct {
	NewRevisionTxn      types.Transaction `json:"newrevisiontxn"`
	NewDownloadSpending types.Currency    `json:"newdownloadspending"`
}

// apply sets the LastRevision, LastRevisionTxn, and DownloadSpending fields
// of the contract being revised.
func (u updateDownloadRevision) apply(data *journalPersist) {
	if len(u.NewRevisionTxn.FileContractRevisions) == 0 {
		build.Critical("updateDownloadRevision is missing its FileContractRevision")
		return
	}
	rev := u.NewRevisionTxn.FileContractRevisions[0]
	c := data.Contracts[rev.ParentID.String()]
	c.LastRevisionTxn = u.NewRevisionTxn
	c.DownloadSpending = u.NewDownloadSpending
	data.Contracts[rev.ParentID.String()] = c
}

// updateCachedUploadRevision is a journalUpdate that records the unsigned
// revision sent to the host during a sector upload, along with the Merkle
// root of the new sector.
type updateCachedUploadRevision struct {
	Revision    types.FileContractRevision `json:"revision"`
	SectorRoot  crypto.Hash                `json:"sectorroot"`
	SectorIndex int                        `json:"sectorindex"`
}

// apply sets the Revision field of the cachedRevision associated with the
// contract being revised, as well as the Merkle root of the new sector.
func (u updateCachedUploadRevision) apply(data *journalPersist) {
	c := data.CachedRevisions[u.Revision.ParentID.String()]
	c.Revision = u.Revision
	if u.SectorIndex == len(c.MerkleRoots) {
		c.MerkleRoots = append(c.MerkleRoots, u.SectorRoot)
	} else if u.SectorIndex < len(c.MerkleRoots) {
		c.MerkleRoots[u.SectorIndex] = u.SectorRoot
	} else {
		// Shouldn't happen. TODO: Add correct error handling.
	}
	data.CachedRevisions[u.Revision.ParentID.String()] = c
}

// updateCachedDownloadRevision is a journalUpdate that records the unsigned
// revision sent to the host during a sector download.
type updateCachedDownloadRevision struct {
	Revision types.FileContractRevision `json:"revision"`
}

// apply sets the Revision field of the cachedRevision associated with the
// contract being revised.
func (u updateCachedDownloadRevision) apply(data *journalPersist) {
	c := data.CachedRevisions[u.Revision.ParentID.String()]
	c.Revision = u.Revision
	data.CachedRevisions[u.Revision.ParentID.String()] = c
}
