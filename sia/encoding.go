package sia

import (
	"encoding/binary"
	"math/big"
	"reflect"
)

// A Marshaler can be encoded to, and decoded from, a byte slice.
// Note: UnmarshalSia may be passed a byte slice containing more than one encoded type.
// It should return the number of bytes used to decode itself.
type Marshaler interface {
	MarshalSia() []byte
	UnmarshalSia([]byte) int
}

// EncInt64 encodes an int64 as a slice of 8 bytes.
func EncInt64(i int64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(i))
	return
}

// DecInt64 decodes a slice of 8 bytes into an int64.
// If len(b) < 8, the slice is padded with zeros.
func DecInt64(b []byte) int64 {
	b2 := b
	if len(b) < 8 {
		b2 = make([]byte, 8)
		copy(b2, b)
	}
	return int64(binary.LittleEndian.Uint64(b2))
}

// EncUint64 encodes a uint64 as a slice of 8 bytes.
func EncUint64(i uint64) (b []byte) {
	b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, i)
	return
}

// DecUint64 decodes a slice of 8 bytes into a uint64.
// If len(b) < 8, the slice is padded with zeros.
func DecUint64(b []byte) uint64 {
	b2 := b
	if len(b) < 8 {
		b2 = make([]byte, 8)
		copy(b2, b)
	}
	return binary.LittleEndian.Uint64(b2)
}

// Marshal encodes a value as a byte slice. The encoding rules are as follows:
// Most types are encoded as their binary representation.
// Integers are little-endian. Int and Uint are disallowed; the number of bits must be specified.
// Variable-length types, such as strings and slices, are prefaced by a single byte containing their length.
// (This may need to be extended to two bytes.)
// Booleans are encoded as one byte, either zero (false) or non-zero (true).
// Nil pointers are represented by a zero.
// Valid pointers are prefaced by a non-zero, followed by the dereferenced value.
func Marshal(v interface{}) []byte {
	return marshal(reflect.ValueOf(v))
}

func marshal(val reflect.Value) (b []byte) {
	// check for MarshalSia interface first
	if m, ok := val.Interface().(Marshaler); ok {
		return m.MarshalSia()
	} else if val.CanAddr() {
		if m, ok := val.Addr().Interface().(Marshaler); ok {
			return m.MarshalSia()
		}
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return []byte{0}
		}
		b = append([]byte{1}, marshal(val.Elem())...)
		return
	case reflect.Bool:
		if val.Bool() {
			return []byte{1}
		} else {
			return []byte{0}
		}
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b := EncInt64(val.Int())
		return b[:val.Type().Bits()/8]
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b := EncUint64(val.Uint())
		return b[:val.Type().Bits()/8]
	case reflect.String:
		s := val.String()
		return append([]byte{byte(len(s))}, []byte(s)...)
	case reflect.Slice: // TODO: add special case for []byte?
		// slices are variable length, so prepend the length and then fallthrough to array logic
		b = []byte{byte(val.Len())}
		fallthrough
	case reflect.Array:
		for i := 0; i < val.Len(); i++ {
			b = append(b, marshal(val.Index(i))...)
		}
		return
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			b = append(b, marshal(val.Field(i))...)
		}
		return
	}
	panic("could not marshal type " + val.Type().String())
	return
}

// Unmarshal decodes a byte slice into the provided interface. The interface must be a pointer.
// The decoding rules are the inverse of those described under Marshal.
func Unmarshal(b []byte, v interface{}) {
	// v must be a pointer
	pval := reflect.ValueOf(v)
	if pval.Kind() != reflect.Ptr || pval.IsNil() {
		panic("Must pass a valid pointer to Unmarshal")
	}
	consumed := unmarshal(b, pval.Elem())
	if consumed != len(b) {
		panic("could not unmarshal type " + pval.Elem().Type().String())
	}
}

func unmarshal(b []byte, val reflect.Value) (consumed int) {
	// check for UnmarshalSia interface first
	if u, ok := val.Interface().(Marshaler); ok {
		return u.UnmarshalSia(b)
	} else if val.CanAddr() {
		if m, ok := val.Addr().Interface().(Marshaler); ok {
			return m.UnmarshalSia(b)
		}
	}

	switch val.Kind() {
	case reflect.Ptr:
		// nil pointer, nothing to decode
		if b[0] == 0 {
			return 1
		}
		// make sure we aren't decoding into nil
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		return unmarshal(b[1:], val.Elem()) + 1
	case reflect.Bool:
		val.SetBool(b[0] != 0)
		return 1
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		size := val.Type().Bits() / 8
		val.SetInt(DecInt64(b[:size]))
		return size
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		size := val.Type().Bits() / 8
		val.SetUint(DecUint64(b[:size]))
		return size
	case reflect.String:
		n, b := int(b[0]), b[1:]
		val.SetString(string(b[:n]))
		return n + 1
	case reflect.Slice: // TODO: add special case for []byte?
		// slices are variable length, but otherwise the same as arrays.
		// just have to allocate them first, then we can fallthrough to the array logic.
		var sliceLen int
		sliceLen, b, consumed = int(b[0]), b[1:], 1 // remember to count the length byte as consumed
		val.Set(reflect.MakeSlice(val.Type(), sliceLen, sliceLen))
		fallthrough
	case reflect.Array:
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			n := unmarshal(b, elem)
			consumed, b = consumed+n, b[n:]
		}
		return
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			f := val.Field(i)
			n := unmarshal(b, f)
			consumed, b = consumed+n, b[n:]
		}
		return
	}
	panic("could not unmarshal type " + val.Type().String())
	return
}

// MarshalSia implements the Marshaler interface for Signatures.
func (s *Signature) MarshalSia() []byte {
	if s.R == nil || s.S == nil {
		return []byte{0, 0}
	}
	// pretend Signature is a tuple of []bytes
	// this lets us use Marshal instead of doing manual length-prefixing
	return Marshal(struct{ R, S []byte }{s.R.Bytes(), s.S.Bytes()})
}

// MarshalSia implements the Unmarshaler interface for Signatures.
func (s *Signature) UnmarshalSia(b []byte) int {
	// inverse of the struct trick used in Signature.MarshalSia
	str := struct{ R, S []byte }{}
	Unmarshal(b, &str)
	s.R = new(big.Int).SetBytes(str.R)
	s.S = new(big.Int).SetBytes(str.S)
	return len(str.R) + len(str.S) + 2
}

// MarshalSia implements the Marshaler interface for PublicKeys.
func (pk *PublicKey) MarshalSia() []byte {
	if pk.X == nil || pk.Y == nil {
		return []byte{0, 0}
	}
	// see Signature.MarshalSia
	return Marshal(struct{ X, Y []byte }{pk.X.Bytes(), pk.Y.Bytes()})
}

// MarshalSia implements the Unmarshaler interface for PublicKeys.
func (pk *PublicKey) UnmarshalSia(b []byte) int {
	// see Signature.UnmarshalSia
	str := struct{ X, Y []byte }{}
	Unmarshal(b, &str)
	pk.X = new(big.Int).SetBytes(str.X)
	pk.Y = new(big.Int).SetBytes(str.Y)
	return len(str.X) + len(str.Y) + 2
}

// MerkleRoot calculates the Merkle root hash of a SpendConditions object,
// using the timelock, number of signatures, and the signatures themselves as leaves.
func (sc *SpendConditions) MerkleRoot() Hash {
	tlHash := HashBytes(Marshal(sc.TimeLock))
	nsHash := HashBytes(Marshal(sc.NumSignatures))
	pkHashes := make([]Hash, len(sc.PublicKeys))
	for i := range sc.PublicKeys {
		pkHashes[i] = HashBytes(Marshal(sc.PublicKeys[i]))
	}
	leaves := append([]Hash{tlHash, nsHash}, pkHashes...)
	return MerkleRoot(leaves)
}
