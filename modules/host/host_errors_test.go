package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/bolt"
)

// persisterErrMkdirAll is a persister that returns an error when MkdirAll is
// called.
type persisterErrMkdirAll struct {
	stub
}

func (persisterErrMkdirAll) MkdirAll(_ string, _ os.FileMode) error {
	return errMkdirAllMock
}

// TestFailedHostMkdirAll initializes the host using a call to MkdirAll that
// will fail.
func TestFailedHostMkdirAll(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestFailedHostMkdirAll")
	if err != nil {
		t.Fatal(err)
	}
	_, err = newHost(persisterErrMkdirAll{}, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != errMkdirAllMock {
		t.Fatal(err)
	}
}

// persisterErrMkdirAll is a persister that returns an error when MkdirAll is
// called.
type persisterErrNewLogger struct {
	stub
}

func (persisterErrNewLogger) NewLogger(_ string) (*persist.Logger, error) {
	return nil, errNewLoggerMock
}

// TestFailedHostNewLogger initializes the host using a call to NewLogger that
// will fail.
func TestFailedHostNewLogger(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := blankHostTester("TestFailedHostNewLogger")
	if err != nil {
		t.Fatal(err)
	}
	_, err = newHost(persisterErrNewLogger{}, ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != errNewLoggerMock {
		t.Fatal(err)
	}
}

// TestUnsuccessfulDBInit sets the stage for an error to be triggered when the
// host tries to initialize the database. The host should return the error.
func TestUnsuccessfulDBInit(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a blank host tester so that all the host dependencies are
	// available.
	ht, err := blankHostTester("TestSetPersistentSettings")
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the host database by deleting BucketStorageObligations, which is
	// used to tell whether the host has been initialized or not. This will
	// cause errors to be returned when initialization tries to create existing
	// buckets.
	err = ht.host.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(BucketStorageObligations)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Close the host so a new host can be created after the database has been
	// corrupted.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(ht.cs, ht.tpool, ht.wallet, ":0", filepath.Join(ht.persistDir, modules.HostDir))
	if err == nil {
		t.Fatal("expecting initDB to fail")
	}
}
