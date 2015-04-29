package encoding

import (
	"errors"
	"fmt"
	"io"
)

var (
	ErrNoData = errors.New("no data")
)

type (
	// A Reader can decode an object from its input. maxLen specifies the
	// maximum length of the object being decoded.
	Reader interface {
		ReadObject(obj interface{}, maxLen uint64) error
	}
	// A Writer can encode objects to its output.
	Writer interface {
		WriteObject(obj interface{}) error
	}
	// A ReadWriter can both read and write objects.
	ReadWriter interface {
		Reader
		Writer
	}
)

// ReadPrefix reads an 8-byte length prefixes, followed by the number of bytes
// specified in the prefix. The operation is aborted if the prefix exceeds a
// specified maximum length.
func ReadPrefix(r io.Reader, maxLen uint64) ([]byte, error) {
	prefix := make([]byte, 8)
	if n, err := io.ReadFull(r, prefix); n == 0 {
		return nil, ErrNoData
	} else if err != nil {
		return nil, errors.New("could not read full length prefix")
	}
	dataLen := DecUint64(prefix)
	if dataLen > maxLen {
		return nil, fmt.Errorf("length %d exceeds maxLen of %d", dataLen, maxLen)
	}
	// read dataLen bytes
	data := make([]byte, dataLen)
	_, err := io.ReadFull(r, data)
	return data, err
}

// ReadObject reads and decodes a length-prefixed and marshalled object.
func ReadObject(r io.Reader, obj interface{}, maxLen uint64) error {
	data, err := ReadPrefix(r, maxLen)
	if err != nil {
		return err
	}
	return Unmarshal(data, obj)
}

// WritePrefix writes a length-prefixed byte slice to w.
func WritePrefix(w io.Writer, data []byte) error {
	n, err := w.Write(append(EncUint64(uint64(len(data))), data...))
	if n != len(data)+8 {
		return io.ErrShortWrite
	}
	return err
}

// WriteObject writes a length-prefixed object to w.
func WriteObject(w io.Writer, v interface{}) error {
	return WritePrefix(w, Marshal(v))
}
