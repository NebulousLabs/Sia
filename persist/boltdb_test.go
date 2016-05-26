package persist

import (
	"math/rand"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/bolt"
)

type testInput struct {
	dbMetadata Metadata
	dbFilename string
}

// TestOpenDatabase tests calling OpenDatabase on the following types of
// database:
// - a database that has not yet been created
// - an existing empty database
// - an existing nonempty database
// Along the way, it also tests calling Close on:
// - a newly-created database
// - a newly-filled database
// - a newly-emptied database
func TestOpenDatabase(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testVersions := []string{"0.0.0", "7.0.4", "asdf"}
	numTestVersions := len(testVersions)

	const numTestInputs = 25
	testInputs := make([]testInput, numTestInputs)

	testBuckets := [][]byte{
		[]byte("FakeBucket"),
		[]byte("FakeBucket123"),
		[]byte("FakeBucket123!@#$"),
		[]byte("Another Fake Bucket"),
		[]byte("FakeBucket" + RandomSuffix()),
		[]byte("_"),
		[]byte(" asdf"),
	}

	// Create a folder for the database file. If a folder by that name exists
	// already, it will be replaced by an empty folder.
	testdir := build.TempDir(persistDir, "TestOpenNewDatabase")
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Generate random testInputs for building database files.
	for i := range testInputs {
		dbFilename := "testFilename" + RandomSuffix()
		dbHeader := "testHeader" + RandomSuffix()
		dbVersion := testVersions[i%numTestVersions]
		dbMetadata := Metadata{dbHeader, dbVersion}
		in := testInput{dbMetadata, dbFilename}
		testInputs[i] = in
	}

	// Loop through tests for each testInput.
	for _, in := range testInputs {
		// Create a new database.
		db, err := OpenDatabase(in.dbMetadata, in.dbFilename)
		if err != nil {
			t.Fatalf("calling OpenDatabase on a new database failed for input %v; error was %v", in, err)
		}

		// Close the newly-created, empty database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly created database failed for input %v; error was %v", in, err)
		}

		// Call OpenDatabase again, this time on the existing empty database.
		db, err = OpenDatabase(in.dbMetadata, in.dbFilename)
		if err != nil {
			t.Fatalf("calling OpenDatabase on an existing empty database failed for input %v; error was %v", in, err)
		}

		// Create buckets in the database.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				_, err := tx.CreateBucketIfNotExists(testBucket)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// Fill each bucket with a random number (0-9, inclusive) of key/value
		// pairs, where each key is a length-10 random byteslice and each value
		// is a length-1000 random byteslice.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				b := tx.Bucket(testBucket)
				x := rand.Intn(10)
				for i := 0; i <= x; i++ {
					k := make([]byte, 10)
					rand.Read(k)
					v := make([]byte, 1e3)
					rand.Read(v)
					err := b.Put(k, v)
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		// Close the newly-filled database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly-filled database failed for input %v; error was %v", in, err)
		}

		// Call OpenDatabase on the database now that it's been filled.
		db, err = OpenDatabase(in.dbMetadata, in.dbFilename)
		if err != nil {
			t.Fatal(err)
		}

		// Empty every bucket in the database.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				b := tx.Bucket(testBucket)
				err := b.ForEach(func(k, v []byte) error {
					return b.Delete(k)
				})
				if err != nil {
					return err
				}
			}
			return nil
		})

		// Close the newly emptied database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly-emptied database failed for input %v; error was %v", in, err)
		}

		// Clean up by deleting the testfile.
		err = os.Remove(in.dbFilename)
		if err != nil {
			t.Fatalf("removing database file failing for input %v; error was %v", in, err)
		}
	}

}
