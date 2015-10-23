package persist

import (
	"errors"
	"time"

	"github.com/NebulousLabs/bolt"
)

type BoltDatabase struct {
	Metadata
	*bolt.DB
}

var (
	ErrNilEntry  = errors.New("entry does not exist")
	ErrNilBucket = errors.New("bucket does not exist")
)

// updateDbMetadata will set the contents of the metadata bucket to be
// what is stored inside the metadata argument
func (db *BoltDatabase) updateMetadata(tx *bolt.Tx) error {
	bucket, err := tx.CreateBucketIfNotExists([]byte("Metadata"))
	if err != nil {
		return err
	}
	err = bucket.Put([]byte("Header"), []byte(db.Header))
	if err != nil {
		return err
	}
	err = bucket.Put([]byte("Version"), []byte(db.Version))
	if err != nil {
		return err
	}
	return nil
}

// checkDbMetadata confirms that the metadata in the database is
// correct. If there is no metadata, correct metadata is inserted
func (db *BoltDatabase) checkMetadata(md Metadata) error {
	err := db.Update(func(tx *bolt.Tx) error {
		// Check if the database has metadata. If not, create metadata for the
		// database.
		bucket := tx.Bucket([]byte("Metadata"))
		if bucket == nil {
			err := db.updateMetadata(tx)
			if err != nil {
				return err
			}
			return nil
		}

		// Verify that the metadata matches the expected metadata.
		header := bucket.Get([]byte("Header"))
		if string(header) != md.Header {
			return ErrBadHeader
		}
		version := bucket.Get([]byte("Version"))
		if string(version) != md.Version {
			return ErrBadVersion
		}
		return nil
	})
	return err
}

// CloseDatabase saves the bolt database to a file, and updates metadata
func (db *BoltDatabase) CloseDatabase() error {
	db.Close()
	return nil
}

// openDatabase opens a database filename and checks metadata
func OpenDatabase(md Metadata, filename string) (*BoltDatabase, error) {
	// Open the database using a 1 second timeout (without the timeout,
	// database will potentially hang indefinitely.
	db, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, err
	}

	boltDB := &BoltDatabase{
		Metadata: md,
		DB:       db,
	}
	err = boltDB.checkMetadata(md)
	if err != nil {
		return nil, err
	}
	return boltDB, nil
}
