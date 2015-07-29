package renter

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"testing"
)

func TestRSEncode(t *testing.T) {
	ecc, err := NewRSCode(10, 3, 130)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 650)
	rand.Read(data)
	r := bytes.NewReader(data)
	buf := new(bytes.Buffer)
	for {
		pieces, err := ecc.Encode(r)
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		err = ecc.Recover(pieces, buf)
		if err != nil {
			t.Fatal(err)
		}
	}
	if bytes.Compare(data, buf.Bytes()) != 0 {
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
	r := bytes.NewReader(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ecc.Encode(r)
	}
}

func BenchmarkRSRecover(b *testing.B) {
	ecc, err := NewRSCode(50, 200, 1<<20)
	if err != nil {
		panic(err)
	}
	pieces, err := ecc.Encode(rand.Reader)
	if err != nil {
		panic(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecc.Recover(pieces, ioutil.Discard)
	}
}
