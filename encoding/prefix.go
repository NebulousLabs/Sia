package encoding

import (
	"errors"
	"fmt"
	"io"
)

// ReadPrefix reads an 8-byte length prefixes, followed by the number of bytes
// specified in the prefix. The operation is aborted if the prefix exceeds a
// specified maximum length.
func ReadPrefix(r io.Reader, maxLen uint64) ([]byte, error) {
	prefix := make([]byte, 8)
	if n, err := r.Read(prefix); err != nil || n != len(prefix) {
		return nil, errors.New("could not read length prefix")
	}
	dataLen := DecUint64(prefix)
	if dataLen > maxLen {
		return nil, fmt.Errorf("length %d exceeds maxLen of %d", dataLen, maxLen)
	}
	// read dataLen bytes
	var data []byte
	buf := make([]byte, 1024)
	var total uint64
	for total = 0; total < dataLen; {
		n, err := r.Read(buf)
		if err != nil {
			return nil, err
		}
		data = append(data, buf[:n]...)
		total += uint64(n)
	}
	if total != dataLen {
		return nil, errors.New("length mismatch")
	}
	return data, nil
}

// ReadObject reads and decodes a length-prefixed and marshalled object.
func ReadObject(r io.Reader, maxLen uint64, obj interface{}) error {
	data, err := ReadPrefix(r, maxLen)
	if err != nil {
		return err
	}
	return Unmarshal(data, obj)
}

// WritePrefix prepends data with a 4-byte length before writing it.
func WritePrefix(w io.Writer, data []byte) (int, error) {
	return w.Write(append(EncUint64(uint64(len(data))), data...))
}

// WriteObject encodes an object and prepends it with a 4-byte length before
// writing it.
func WriteObject(w io.Writer, obj interface{}) (int, error) {
	return WritePrefix(w, Marshal(obj))
}
