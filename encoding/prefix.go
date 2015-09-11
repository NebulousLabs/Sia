package encoding

import (
	"errors"
	"fmt"
	"io"
)

var (
	errNoData    = errors.New("no data")
	errBadPrefix = errors.New("could not read full length prefix")
)

// ReadPrefix reads an 8-byte length prefixes, followed by the number of bytes
// specified in the prefix. The operation is aborted if the prefix exceeds a
// specified maximum length.
func ReadPrefix(r io.Reader, maxLen uint64) ([]byte, error) {
	prefix := make([]byte, 8)
	if n, err := io.ReadFull(r, prefix); n == 0 {
		return nil, errNoData
	} else if err != nil {
		return nil, errBadPrefix
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

// AddPrefix prepends b with an 8-byte length prefix.
func AddPrefix(b []byte) []byte {
	return append(EncUint64(uint64(len(b))), b...)
}

// WritePrefix writes a length-prefixed byte slice to w.
func WritePrefix(w io.Writer, data []byte) error {
	n, err := w.Write(AddPrefix(data))
	if n != len(data)+8 {
		return io.ErrShortWrite
	}
	return err
}

// WriteObject writes a length-prefixed object to w.
func WriteObject(w io.Writer, v interface{}) error {
	return WritePrefix(w, Marshal(v))
}
