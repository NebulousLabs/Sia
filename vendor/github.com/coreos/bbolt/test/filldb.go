package main

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"github.com/y0ssar1an/q"

	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/fastrand"

	"net/http"
	_ "net/http/pprof"
)

var db *bolt.DB
var globalTx *bolt.Tx
var mu sync.Mutex

func syncDB() {
	// Commit the existing tx.
	fmt.Println("committing")
	err := globalTx.Commit()
	if err != nil {
		panic(err)
	}
	q.Q(globalTx.Stats())
	globalTx.Rollback()
	globalTx, err = db.Begin(true)
	if err != nil {
		panic(err)
	}
}

var keys = make([][32]byte, 5000)
var v = make([]byte, 128)

func insertKeys(tx *bolt.Tx, i int) {
	b, err := tx.CreateBucketIfNotExists([]byte("foo"))
	if err != nil {
		panic(err)
	}

	for i := range keys {
		fastrand.Read(keys[i][:])
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) > 0
	})
	for _, k := range keys {
		fastrand.Read(v)
		b.Put(k[:], v)
		b.Get(v[:32])
	}
}

func main() {
	//	defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()

	go http.ListenAndServe(":8080", nil)

	var err error
	db, err = bolt.Open("big.db", 0666, nil)
	if err != nil {
		panic(err)
	}
	//db.NoSync = true

	for i := 0; i < 100; i++ {
		tx, err := db.Begin(true)
		if err != nil {
			panic(err)
		}

		fmt.Print(i, " ")
		insertKeys(tx, i)
		err = tx.Commit()
		if err != nil {
			panic(err)
		}
	}
}

// unsorted, sync:   17.5s
// unsorted, nosync:  7.7s
// sorted, sync:
// sorted, nosync:
