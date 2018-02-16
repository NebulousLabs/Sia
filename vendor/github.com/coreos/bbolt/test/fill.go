package main

import (
	"log"
	"math/rand"

	"github.com/NebulousLabs/bolt"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func main() {
	db, err := bolt.Open("test.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for {
		err = db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucket(randBytes(20))
			if err != nil {
				return err
			}
			for i := 0; i < 5000; i++ {
				err := b.Put(randBytes(500), randBytes(500))
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}
