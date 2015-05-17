package encoding

import (
	"bytes"
	"io"
	"testing"
)

func TestReadPrefix(t *testing.T) {
	b := new(bytes.Buffer)

	// standard
	b.Write(append(EncUint64(3), "foo"...))
	data, err := ReadPrefix(b, 3)
	if err != nil {
		t.Error(err)
	} else if string(data) != "foo" {
		t.Errorf("expected foo, got %s", data)
	}

	// 0-length
	b.Write(EncUint64(0))
	_, err = ReadPrefix(b, 0)
	if err != nil {
		t.Error(err)
	}

	// empty
	b.Write([]byte{})
	_, err = ReadPrefix(b, 3)
	if err != ErrNoData {
		t.Error("expected ErrNoData, got", err)
	}

	// less than 8 bytes
	b.Write([]byte{1, 2, 3})
	_, err = ReadPrefix(b, 3)
	if err != ErrBadPrefix {
		t.Error("expected ErrBadPrefix, got", err)
	}

	// exceed maxLen
	b.Write(EncUint64(4))
	_, err = ReadPrefix(b, 3)
	if err == nil {
		t.Error("expected ErrBadPrefix, got nil")
	} else if err.Error() != "length 4 exceeds maxLen of 3" {
		t.Error("expected maxLen error, got", err)
	}

	// no data after length prefix
	b.Write(EncUint64(3))
	_, err = ReadPrefix(b, 3)
	if err != io.EOF {
		t.Error("expected EOF, got", err)
	}
}

func TestReadObject(t *testing.T) {
	b := new(bytes.Buffer)
	var obj string

	// standard
	b.Write(EncUint64(11))
	b.Write(append(EncUint64(3), "foo"...))
	err := ReadObject(b, &obj, 11)
	if err != nil {
		t.Error(err)
	} else if obj != "foo" {
		t.Errorf("expected foo, got %s", obj)
	}

	// empty
	b.Write([]byte{})
	err = ReadObject(b, &obj, 0)
	if err != ErrNoData {
		t.Error("expected ErrNoData, got", err)
	}

	// bad object
	b.Write(EncUint64(3))
	b.WriteString("foo") // strings need an additional length prefix
	err = ReadObject(b, &obj, 3)
	if err == nil {
		t.Error("expected err, got nil")
	} else if err.Error() != "could not decode type string: "+ErrBadPrefix.Error() {
		t.Error("expected ErrBadPrefix, got", err)
	}
}

type badWriter struct{}

func (bw *badWriter) Write([]byte) (int, error) { return 0, nil }

func TestWritePrefix(t *testing.T) {
	b := new(bytes.Buffer)

	// standard
	err := WritePrefix(b, []byte("foo"))
	expected := append(EncUint64(3), "foo"...)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(b.Bytes(), expected) != 0 {
		t.Error("WritePrefix wrote wrong data: expected %v, got %v", b.Bytes(), expected)
	}

	// badWriter (returns nil error, but doesn't write anything)
	bw := new(badWriter)
	err = WritePrefix(bw, []byte("foo"))
	if err != io.ErrShortWrite {
		t.Error("expected ErrShortWrite, got", err)
	}
}

func TestWriteObject(t *testing.T) {
	b := new(bytes.Buffer)

	// standard
	err := WriteObject(b, "foo")
	expected := append(EncUint64(11), append(EncUint64(3), "foo"...)...)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(b.Bytes(), expected) != 0 {
		t.Error("WritePrefix wrote wrong data: expected %v, got %v", b.Bytes(), expected)
	}

	// badWriter
	bw := new(badWriter)
	err = WriteObject(bw, "foo")
	if err != io.ErrShortWrite {
		t.Error("expected ErrShortWrite, got", err)
	}
}

// feed writers into readers
func TestReadWrite(t *testing.T) {
	b := new(bytes.Buffer)

	// WritePrefix -> ReadPrefix
	data := []byte("foo")
	err := WritePrefix(b, data)
	if err != nil {
		t.Fatal(err)
	}
	rdata, err := ReadPrefix(b, 100)
	if err != nil {
		t.Error(err)
	} else if bytes.Compare(rdata, data) != 0 {
		t.Errorf("read/write mismatch: wrote %s, read %s", data, rdata)
	}

	// WriteObject -> ReadObject
	obj := "bar"
	err = WriteObject(b, obj)
	if err != nil {
		t.Fatal(err)
	}
	var robj string
	err = ReadObject(b, &robj, 100)
	if err != nil {
		t.Error(err)
	} else if robj != obj {
		t.Errorf("read/write mismatch: wrote %s, read %s", obj, robj)
	}
}
