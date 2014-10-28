package sia

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func EncUint64(i uint64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, i)
	return
}

func MarshalAll(data ...interface{}) []byte {
	buf := new(bytes.Buffer)
	var enc []byte
	for i := range data {
		switch d := data[i].(type) {
		case []byte:
			enc = d
		case string:
			enc = []byte(d)
		case uint64:
			enc = EncUint64(d)
		case Hash:
			enc = d[:]
		// more to come
		default:
			panic(fmt.Sprintf("can't marshal type %T", d))
		}
		buf.Write(enc)
	}
	return buf.Bytes()
}
