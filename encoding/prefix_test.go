package encoding

import (
	"bytes"
	"io"
	"testing"
)

// badReader/Writer used to test error handling

type badReader struct{}

func (br *badReader) Read([]byte) (int, error) { return 0, io.EOF }

type badWriter struct{}

func (bw *badWriter) Write([]byte) (int, error) { return 0, nil }

func TestReadPrefixedBytes(t *testing.T) {
	b := new(bytes.Buffer)

	// standard
	b.Write(append(EncUint64(3), "foo"...))
	data, err := ReadPrefixedBytes(b, 3)
	if err != nil {
		t.Error(err)
	} else if string(data) != "foo" {
		t.Errorf("expected foo, got %s", data)
	}

	// 0-length
	b.Write(EncUint64(0))
	_, err = ReadPrefixedBytes(b, 0)
	if err != nil {
		t.Error(err)
	}

	// empty
	b.Write([]byte{})
	_, err = ReadPrefixedBytes(b, 3)
	if err != io.EOF {
		t.Error("expected EOF, got", err)
	}

	// less than 8 bytes
	b.Write([]byte{1, 2, 3})
	_, err = ReadPrefixedBytes(b, 3)
	if err != io.ErrUnexpectedEOF {
		t.Error("expected unexpected EOF, got", err)
	}

	// exceed maxLen
	b.Write(EncUint64(4))
	_, err = ReadPrefixedBytes(b, 3)
	if err == nil || err.Error() != "length 4 exceeds maxLen of 3" {
		t.Error("expected maxLen error, got", err)
	}

	// no data after length prefix
	b.Write(EncUint64(3))
	_, err = ReadPrefixedBytes(b, 3)
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
	if err != io.EOF {
		t.Error("expected EOF, got", err)
	}

	// bad object
	b.Write(EncUint64(3))
	b.WriteString("foo") // strings need an additional length prefix
	err = ReadObject(b, &obj, 3)
	if err == nil || err.Error() != "could not decode type string: "+io.ErrUnexpectedEOF.Error() {
		t.Error("expected unexpected EOF, got", err)
	}
}

func TestWritePrefixedBytes(t *testing.T) {
	b := new(bytes.Buffer)

	// standard
	err := WritePrefixedBytes(b, []byte("foo"))
	expected := append(EncUint64(3), "foo"...)
	if err != nil {
		t.Error(err)
	} else if !bytes.Equal(b.Bytes(), expected) {
		t.Errorf("WritePrefixedBytes wrote wrong data: expected %v, got %v", b.Bytes(), expected)
	}

	// badWriter (returns nil error, but doesn't write anything)
	bw := new(badWriter)
	err = WritePrefixedBytes(bw, []byte("foo"))
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
	} else if !bytes.Equal(b.Bytes(), expected) {
		t.Errorf("WritePrefixedBytes wrote wrong data: expected %v, got %v", b.Bytes(), expected)
	}

	// badWriter
	bw := new(badWriter)
	err = WriteObject(bw, "foo")
	if err != io.ErrShortWrite {
		t.Error("expected ErrShortWrite, got", err)
	}
}

func TestReadWritePrefixedBytes(t *testing.T) {
	b := new(bytes.Buffer)

	// WritePrefixedBytes -> ReadPrefixedBytes
	data := []byte("foo")
	err := WritePrefixedBytes(b, data)
	if err != nil {
		t.Fatal(err)
	}
	rdata, err := ReadPrefixedBytes(b, 100)
	if err != nil {
		t.Error(err)
	} else if !bytes.Equal(rdata, data) {
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
