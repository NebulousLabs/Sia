package sia

import (
	"encoding/binary"
)

func EncUint32(i uint32) (b []byte) {
	b = make([]byte, 4)
	binary.LittleEndian.PutUint32(b, i)
	return
}
