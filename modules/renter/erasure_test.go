package renter

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"testing"
)

func TestRSEncode(t *testing.T) {
	ecc, err := NewRSCode(10, 3)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 777)
	rand.Read(data)

	pieces, err := ecc.Encode(data)
	if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	err = ecc.Recover(pieces, 777, buf)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, buf.Bytes()) {
		t.Fatal("recovered data does not match original")
	}
}

func BenchmarkRSEncode(b *testing.B) {
	ecc, err := NewRSCode(80, 20)
	if err != nil {
		panic(err)
	}
	data := make([]byte, 1<<20)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecc.Encode(data)
	}
}

func BenchmarkRSRecover(b *testing.B) {
	ecc, err := NewRSCode(50, 200)
	if err != nil {
		panic(err)
	}
	data := make([]byte, 1<<20)
	rand.Read(data)
	pieces, err := ecc.Encode(data)
	if err != nil {
		panic(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecc.Recover(pieces, 1<<20, ioutil.Discard)
	}
}
