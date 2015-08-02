package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"testing"
)

func TestRSEncode(t *testing.T) {
	ecc, err := NewRSCode(10, 3, 1024)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 777)
	rand.Read(data)

	pieces, err := ecc.Encode(data)
	if err == io.EOF {
		break
	} else if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	err = ecc.Recover(pieces, buf)
	if err != nil {
		t.Fatal(err)
	}

	buf.Truncate(777)
	if !bytes.Equal(data, buf.Bytes()) {
		t.Fatal("recovered data does not match original")
	}
}

func BenchmarkRSEncode(b *testing.B) {
	ecc, err := NewRSCode(80, 20, 1<<20) // 1 MB
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
	ecc, err := NewRSCode(50, 200, 1<<20)
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
		ecc.Recover(pieces, ioutil.Discard)
	}
}
