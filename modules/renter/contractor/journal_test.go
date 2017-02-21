package contractor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

func tempFile(t interface {
	Fatal(...interface{})
}, name string) (*os.File, func()) {
	f, err := os.Create(filepath.Join(build.TempDir("contractor", name)))
	if err != nil {
		t.Fatal(err)
	}
	return f, func() {
		f.Close()
		os.RemoveAll(f.Name())
	}
}

func tempJournal(t interface {
	Fatal(...interface{})
}, name string) (*journal, func()) {
	j, err := newJournal(filepath.Join(build.TempDir("contractor", name)))
	if err != nil {
		t.Fatal(err)
	}
	return j, func() {
		j.Close()
		os.RemoveAll(j.filename)
	}
}

func TestJournal(t *testing.T) {
	j, cleanup := tempJournal(t, "TestJournal")
	defer cleanup()

	us := []journalUpdate{
		updateCachedDownloadRevision{Revision: types.FileContractRevision{}},
	}
	if err := j.update(us); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	var data contractorPersist
	j2, err := openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if len(data.CachedRevisions) != 1 {
		t.Fatal("openJournal applied updates incorrectly:", data)
	}
}

func TestJournalCheckpoint(t *testing.T) {
	j, cleanup := tempJournal(t, "TestJournalCheckpoint")
	defer cleanup()

	var data contractorPersist
	data.BlockHeight = 777
	if err := j.checkpoint(data); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	data.BlockHeight = 0
	j2, err := openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if data.BlockHeight != 777 {
		t.Fatal("checkpoint failed:", data)
	}
}

func TestJournalMalformed(t *testing.T) {
	f, cleanup := tempFile(t, "TestJournalMalformed")
	defer cleanup()

	// write a partially-malformed log
	f.WriteString(`{"cachedrevisions":{}}
[{"t":"cachedDownloadRevision","d":{"revision":{"parentid":"1000000000000000000000000000000000000000000000000000000000000000"}}}]
[{"t":"cachedDownloadRevision","d":{"revision":{"parentid":"2000000000000000000000000000000000000000000000000000000000000000"
`)
	f.Close()

	// load log
	var data contractorPersist
	j, err := openJournal(f.Name(), &data)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()

	// the last update set should have been discarded
	if _, ok := data.CachedRevisions["1000000000000000000000000000000000000000000000000000000000000000"]; !ok {
		t.Fatal("log was not applied correctly:", data)
	}
}

func BenchmarkUpdateJournal(b *testing.B) {
	f, cleanup := tempFile(b, "BenchmarkUpdateJournal")
	defer cleanup()

	j := &journal{f: f}
	us := updateSet{
		updateCachedUploadRevision{
			Revision: types.FileContractRevision{
				NewValidProofOutputs:  []types.SiacoinOutput{{}, {}},
				NewMissedProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions:      types.UnlockConditions{PublicKeys: []types.SiaPublicKey{{}, {}}},
			},
		},
		updateUploadRevision{
			NewRevisionTxn: types.Transaction{
				FileContractRevisions: []types.FileContractRevision{{
					NewValidProofOutputs:  []types.SiacoinOutput{{}, {}},
					NewMissedProofOutputs: []types.SiacoinOutput{{}, {}},
					UnlockConditions:      types.UnlockConditions{PublicKeys: []types.SiaPublicKey{{}, {}}},
				}},
				TransactionSignatures: []types.TransactionSignature{{}, {}},
			},
			NewUploadSpending:  types.SiacoinPrecision,
			NewStorageSpending: types.SiacoinPrecision,
		},
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(us)
	b.SetBytes(int64(buf.Len()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := j.update(us); err != nil {
			b.Fatal(err)
		}
	}
}
