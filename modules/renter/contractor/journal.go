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
	"io"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var journalMeta = persist.Metadata{
	Header:  "Contractor Journal",
	Version: "1.0.0",
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

// Checkpoint refreshes the journal with a new initial object. It syncs the
// underlying file before returning.
func (j *journal) checkpoint(data contractorPersist) error {
	// write to a new temp file
	tmp, err := os.Create(j.filename + "_tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(journalMeta); err != nil {
		return err
	}
	if err := enc.Encode(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}

	// atomically replace the old file with the new one
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := j.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), j.filename); err != nil {
		return err
	}

	// reopen the journal
	j.f, err = os.OpenFile(j.filename, os.O_RDWR|os.O_APPEND, 0)
	return err
}

// Close closes the underlying file.
func (j *journal) Close() error {
	return j.f.Close()
}

// newJournal creates a new journal, using an empty contractorPersist as the
// initial object.
func newJournal(filename string) (*journal, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	// write metadata
	enc := json.NewEncoder(f)
	if err := enc.Encode(journalMeta); err != nil {
		return nil, err
	}
	// write empty contractorPersist
	if err := enc.Encode(contractorPersist{}); err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, err
	}
	return &journal{f: f, filename: filename}, nil
}

// openJournal opens the supplied journal and decodes the reconstructed
// contractorPersist into data.
func openJournal(filename string, data *contractorPersist) (*journal, error) {
	// open file handle for reading and writing
	f, err := os.OpenFile(filename, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	// decode metadata
	dec := json.NewDecoder(f)
	var meta persist.Metadata
	if err = dec.Decode(&meta); err != nil {
		return nil, err
	} else if meta.Version != journalMeta.Version {
		return nil, errors.New("incompatible version")
	}

	// decode the initial object
	if err = dec.Decode(data); err != nil {
		return nil, err
	}

	// make sure all maps are properly initialized
	if data.CachedRevisions == nil {
		data.CachedRevisions = map[string]cachedRevision{}
	}
	if data.Contracts == nil {
		data.Contracts = map[string]modules.RenterContract{}
	}
	if data.RenewedIDs == nil {
		data.RenewedIDs = map[string]string{}
	}

	// decode each set of updates and apply them to data
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
	apply(*contractorPersist)
}

type marshaledUpdate struct {
	Type     string      `json:"t"`
	Data     rawJSON     `json:"d"`
	Checksum crypto.Hash `json:"c"`
}

// TODO: replace with json.RawMessage after upgrading to Go 1.8
type rawJSON []byte

// MarshalJSON returns r as the JSON encoding of r.
func (r rawJSON) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return r, nil
}

// UnmarshalJSON sets *r to a copy of data.
func (r *rawJSON) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("rawJSON: UnmarshalJSON on nil pointer")
	}
	*r = append((*r)[:0], data...)
	return nil
}

type updateSet []journalUpdate

func (set updateSet) MarshalJSON() ([]byte, error) {
	marshaledSet := make([]marshaledUpdate, len(set))
	for i, u := range set {
		data, err := json.Marshal(u)
		if err != nil {
			build.Critical("failed to marshal known type:", err)
		}
		marshaledSet[i].Data = data
		marshaledSet[i].Checksum = crypto.HashBytes(data)
		switch u.(type) {
		case updateUploadRevision:
			marshaledSet[i].Type = "uploadRevision"
		case updateDownloadRevision:
			marshaledSet[i].Type = "downloadRevision"
		case updateCachedUploadRevision:
			marshaledSet[i].Type = "cachedUploadRevision"
		case updateCachedDownloadRevision:
			marshaledSet[i].Type = "cachedDownloadRevision"
		}
	}
	return json.Marshal(marshaledSet)
}

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

type updateUploadRevision struct {
	NewRevisionTxn     types.Transaction `json:"newrevisiontxn"`
	NewSectorRoot      crypto.Hash       `json:"newsectorroot"`
	NewUploadSpending  types.Currency    `json:"newuploadspending"`
	NewStorageSpending types.Currency    `json:"newstoragespending"`
}

func (u updateUploadRevision) apply(data *contractorPersist) {
	if len(u.NewRevisionTxn.FileContractRevisions) == 0 {
		return // shouldn't happen
	}
	rev := u.NewRevisionTxn.FileContractRevisions[0]
	c := data.Contracts[rev.ParentID.String()]
	c.LastRevisionTxn = u.NewRevisionTxn
	c.LastRevision = rev
	c.MerkleRoots = append(c.MerkleRoots, u.NewSectorRoot) // TODO: make this idempotent
	c.UploadSpending = u.NewUploadSpending
	c.StorageSpending = u.NewStorageSpending
	data.Contracts[rev.ParentID.String()] = c
}

type updateDownloadRevision struct {
	NewRevisionTxn      types.Transaction `json:"newrevisiontxn"`
	NewDownloadSpending types.Currency    `json:"newdownloadspending"`
}

func (u updateDownloadRevision) apply(data *contractorPersist) {
	if len(u.NewRevisionTxn.FileContractRevisions) == 0 {
		return // shouldn't happen
	}
	rev := u.NewRevisionTxn.FileContractRevisions[0]
	c := data.Contracts[rev.ParentID.String()]
	c.LastRevisionTxn = u.NewRevisionTxn
	c.LastRevision = rev
	c.DownloadSpending = u.NewDownloadSpending
	data.Contracts[rev.ParentID.String()] = c
}

type updateCachedUploadRevision struct {
	Revision   types.FileContractRevision `json:"revision"`
	SectorRoot crypto.Hash                `json:"sectorroot"`
}

func (u updateCachedUploadRevision) apply(data *contractorPersist) {
	c := data.CachedRevisions[u.Revision.ParentID.String()]
	c.Revision = u.Revision
	c.MerkleRoots = append(c.MerkleRoots, u.SectorRoot) // TODO: make this idempotent
	data.CachedRevisions[u.Revision.ParentID.String()] = c
}

type updateCachedDownloadRevision struct {
	Revision types.FileContractRevision `json:"revision"`
}

func (u updateCachedDownloadRevision) apply(data *contractorPersist) {
	c := data.CachedRevisions[u.Revision.ParentID.String()]
	c.Revision = u.Revision
	data.CachedRevisions[u.Revision.ParentID.String()] = c
}
