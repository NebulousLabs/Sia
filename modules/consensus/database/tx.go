package database

import "github.com/coreos/bbolt"

// A Tx is a database transaction.
type Tx interface {
	Bucket(name []byte) *bolt.Bucket
	CreateBucket(name []byte) (*bolt.Bucket, error)
	CreateBucketIfNotExists(name []byte) (*bolt.Bucket, error)
	DeleteBucket(name []byte) error
	ForEach(func([]byte, *bolt.Bucket) error) error
}

type txWrapper struct {
	*bolt.Tx
}
