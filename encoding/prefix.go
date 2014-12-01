package encoding

import (
	"errors"
	"fmt"
	"io"
)

// ReadPrefix reads a 4-byte length prefixes, followed by the number of bytes
// specified in the prefix. The operation is aborted if the prefix exceeds a
// specified maximum length.
func ReadPrefix(r io.Reader, maxLen uint32) ([]byte, error) {
	prefix := make([]byte, 4)
	if n, err := r.Read(prefix); err != nil || n != len(prefix) {
		return nil, errors.New("could not read length prefix")
	}
	dataLen := DecLen(prefix)
	if uint32(dataLen) > maxLen {
		return nil, fmt.Errorf("length %d exceeds maxLen of %d", dataLen, maxLen)
	}
	// read dataLen bytes
	var data []byte
	buf := make([]byte, 1024)
	for total := 0; total < dataLen; {
		n, err := r.Read(buf)
		if err != nil {
			return nil, err
		}
		data = append(data, buf[:n]...)
		total += n
	}
	if len(data) != dataLen {
		return nil, errors.New("length mismatch")
	}
	return data, nil
}

// ReadObject reads and decodes a length-prefixed and marshalled object.
func ReadObject(r io.Reader, maxLen uint32, obj interface{}) error {
	data, err := ReadPrefix(r, maxLen)
	if err != nil {
		return err
	}
	return Unmarshal(data, obj)
}

// WritePrefix prepends data with a 4-byte length before writing it.
func WritePrefix(w io.Writer, data []byte) (int, error) {
	return w.Write(append(EncLen(len(data)), data...))
}

// WriteObject encodes an object and prepends it with a 4-byte length before
// writing it.
func WriteObject(w io.Writer, obj interface{}) (int, error) {
	return WritePrefix(w, Marshal(obj))
}
