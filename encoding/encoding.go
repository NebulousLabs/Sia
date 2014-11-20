package encoding

import (
	"encoding/binary"
	"errors"
	"reflect"
)

// A Marshaler can be encoded as a byte slice.
// Marshaler and Unmarshaler are separate interfaces because Unmarshaler must
// have a pointer receiver, while Marshaler does not.
//
// SHOULD PROBABLY HAVE A DIFFERENT NAME. SIAMARSHALLER IF ALL ELSE FAILS.
type Marshaler interface {
	MarshalSia() []byte
}

// An Unmarshaler can be decoded from a byte slice.
// UnmarshalSia may be passed a byte slice containing more than one encoded type.
// It should return the number of bytes used to decode itself.
//
// SHOULD PROBABLY HAVE A DIFFERENT NAME. SIAUNMARSHALLER IF ALL ELSE FAILS.
type Unmarshaler interface {
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
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return EncInt64(val.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return EncUint64(val.Uint())
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
	// Marshalling should never fail. If it panics, you're doing something wrong,
	// like trying to encode an int or a map or an unexported struct field.
	panic("could not marshal type " + val.Type().String())
	return
}

// Unmarshal decodes a byte slice into the provided interface. The interface must be a pointer.
// The decoding rules are the inverse of those described under Marshal.
func Unmarshal(b []byte, v interface{}) (err error) {
	// v must be a pointer
	pval := reflect.ValueOf(v)
	if pval.Kind() != reflect.Ptr || pval.IsNil() {
		return errors.New("must pass a valid pointer to Unmarshal")
	}

	// unmarshal may panic
	var consumed int
	defer func() {
		if r := recover(); r != nil || consumed != len(b) {
			err = errors.New("could not unmarshal type " + pval.Elem().Type().String())
		}
	}()

	consumed = unmarshal(b, pval.Elem())
	return
}

func unmarshal(b []byte, val reflect.Value) (consumed int) {
	// check for UnmarshalSia interface first
	if u, ok := val.Interface().(Unmarshaler); ok {
		return u.UnmarshalSia(b)
	} else if val.CanAddr() {
		if m, ok := val.Addr().Interface().(Unmarshaler); ok {
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
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val.SetInt(DecInt64(b[:8]))
		return 8
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val.SetUint(DecUint64(b[:8]))
		return 8
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
	panic("unknown type")
	return
}

// MarshalAll marshals all of its inputs and returns their concatenation.
func MarshalAll(v ...interface{}) (b []byte) {
	for i := range v {
		b = append(b, Marshal(v[i])...)
	}
	return
}
