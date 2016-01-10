package renter

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"testing"
)

// TestRSEncode tests the rsCode type.
func TestRSEncode(t *testing.T) {
	badParams := []struct {
		data, parity int
	}{
		{-1, -1},
		{-1, 0},
		{0, -1},
		{0, 0},
		{0, 1},
		{1, 0},
	}
	for _, ps := range badParams {
		if _, err := NewRSCode(ps.data, ps.parity); err == nil {
			t.Error("expected bad parameter error, got nil")
		}
	}

	rsc, err := NewRSCode(10, 3)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 777)
	rand.Read(data)

	pieces, err := rsc.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rsc.Encode(nil)
	if err == nil {
		t.Fatal("expected nil data error, got nil")
	}

	buf := new(bytes.Buffer)
	err = rsc.Recover(pieces, 777, buf)
	if err != nil {
		t.Fatal(err)
	}
	err = rsc.Recover(nil, 777, buf)
	if err == nil {
		t.Fatal("expected nil pieces error, got nil")
	}

	if !bytes.Equal(data, buf.Bytes()) {
		t.Fatal("recovered data does not match original")
	}
}

func BenchmarkRSEncode(b *testing.B) {
	rsc, err := NewRSCode(80, 20)
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, 1<<20)
	rand.Read(data)

	b.SetBytes(1 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rsc.Encode(data)
	}
}

func BenchmarkRSRecover(b *testing.B) {
	rsc, err := NewRSCode(50, 200)
	if err != nil {
		b.Fatal(err)
	}
	data := make([]byte, 1<<20)
	rand.Read(data)
	pieces, err := rsc.Encode(data)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(1 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pieces[0] = nil
		rsc.Recover(pieces, 1<<20, ioutil.Discard)
	}
}
