package sia

import (
	"bytes"
	"crypto/rand"
)

// Hashcash brute-forces a nonce that produces a hash less than target.
func Hashcash(target Hash) (nonce []byte, i int) {
	nonce = make([]byte, 8)
	for {
		i++
		rand.Read(nonce)
		h := HashBytes(nonce)
		if bytes.Compare(h[:], target[:]) < 0 {
			return
		}
	}
}
