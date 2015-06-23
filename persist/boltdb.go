package persist

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/boltdb/bolt"
)

// See json.go for definitions of errors and metadata

type BoltDatabase struct {
	*bolt.DB
	meta Metadata
}

type BoltItem struct {
	BucketName string
	Key        []byte
	Value      []byte
}

// checkDbMetadata confirms that the metadata in the database is
// correct. If there is no metadata, correct metadata is inserted
func (db *BoltDatabase) checkMetadata(meta Metadata) error {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("Metadata"))
		if bucket == nil {
			err := db.updateMetadata(tx)
			if err != nil {
				return err
			}
		}

		// Get bucket in the case that it has just been made
		// Doing this twice should have no ill effect
		bucket = tx.Bucket([]byte("Metadata"))

		header := bucket.Get([]byte("Header"))
		if string(header) != meta.Header {
			return ErrBadHeader
		}

		version := bucket.Get([]byte("Version"))
		if string(version) != meta.Version {
			return ErrBadVersion
		}
		return nil
	})
	return err
}

// updateDbMetadata will set the contents of the metadata bucket to be
// what is stored inside the metadata argument
func (db *BoltDatabase) updateMetadata(tx *bolt.Tx) error {
	bucket, err := tx.CreateBucketIfNotExists([]byte("Metadata"))
	if err != nil {
		return err
	}

	err = bucket.Put([]byte("Header"), []byte(db.meta.Header))
	if err != nil {
		return err
	}

	err = bucket.Put([]byte("Version"), []byte(db.meta.Version))
	if err != nil {
		return err
	}
	return nil
}

// A generic function to insert a value into a specific bucket in the database
func (db *BoltDatabase) InsertIntoBucket(bucketName string, key []byte, value []byte) error {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if build.DEBUG {
			if bucket == nil {
				panic(bucketName + "bucket was not created correcty")
			}
		}

		err := bucket.Put(key, value)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

// insertIntoBuckets is a generic function to insert many items many
// buckets
func (db *BoltDatabase) BulkInsert(items []BoltItem) error {
	err := db.Update(func(tx *bolt.Tx) error {
		for _, item := range items {
			bucket := tx.Bucket([]byte(item.BucketName))
			if bucket == nil {
				return errors.New("requested bucket does not exist: " + item.BucketName)
			}

			err := bucket.Put(item.Key, item.Value)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// BulkGet retrieves many items from the database in the same transaction
func (db *BoltDatabase) BulkGet(items []BoltItem) ([][]byte, error) {
	values := make([][]byte, len(items))
	err := db.View(func(tx *bolt.Tx) error {
		for i, item := range items {
			bucket := tx.Bucket([]byte(item.BucketName))
			if bucket == nil {
				return errors.New("requested bucket does not exist: " + item.BucketName)
			}

			values[i] = bucket.Get(item.Key)
			if values[i] == nil {
				return errors.New(fmt.Sprintf("requested item %x does not exist in bucket %s",
					item.Key,
					item.BucketName))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return values, nil
}

// closeDatabase saves the bolt database to a file, and updates metadata
func (db *BoltDatabase) CloseDatabase() error {
	err := db.Update(db.updateMetadata)
	if err != nil {
		return err
	}

	db.Close()
	return nil
}

// openDatabase opens a database filename and checks metadata
func OpenDatabase(meta Metadata, filename string) (*BoltDatabase, error) {
	db, err := bolt.Open(filename, 0600, nil)
	if err != nil {
		return nil, err
	}

	boltDB := &BoltDatabase{db, meta}

	err = boltDB.checkMetadata(meta)
	if err != nil {
		return nil, err
	}
	return boltDB, nil
}
