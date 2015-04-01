package encoding

import (
	"errors"
	"reflect"
)

// A Marshaler can be encoded as a byte slice. (Note that Marshaler and
// Unmarshaler are separate interfaces because Unmarshaler must have a pointer
// receiver, while Marshaler does not.)
type SiaMarshaler interface {
	MarshalSia() []byte
}

// An Unmarshaler can be decoded from a byte slice. If a decoding error occurs,
// UnmarshalSia should panic.
type SiaUnmarshaler interface {
	UnmarshalSia([]byte)
}

// Marshal encodes a value as a byte slice. The encoding rules are as follows:
//
// Most types are encoded as their binary representation.
//
// Integers are little-endian, and are always encoded as 8 bytes, i.e. their
// int64 or uint64 equivalent.
//
// Booleans are encoded as one byte, either zero (false) or non-zero (true).
//
// Nil pointers are represented by a zero.
//
// Valid pointers are prefaced by a non-zero, followed by the dereferenced
// value.
//
// Variable-length types, such as strings and slices, are prefaced by 8 bytes
// containing their length.
//
// Slices and structs are simply the concatenation of their encoded elements.
// Byte slices are not subject to the 8-byte integer rule; they are encoded as
// their literal representation, one byte per byte.  The ordering of struct
// fields is determined by their type definition. For example:
//
//   type foo struct { S string I int }
//
//   Marshal(foo{"bar", 3}) = append(Marshal("bar"), Marshal(3)...)
//
// Finally, if a type implements the SiaMarshaler interface, its MarshalSia
// method will be used to encode the type. The resulting byte slice will be
// length-prefixed like any other variable-length types. During decoding, this
// prefix is used to determine how many bytes should be passed to UnmarshalSia.
func Marshal(v interface{}) []byte {
	return marshal(reflect.ValueOf(v))
}

func marshal(val reflect.Value) (b []byte) {
	// check for MarshalSia interface first
	if m, ok := val.Interface().(SiaMarshaler); ok {
		data := m.MarshalSia()
		return append(EncUint64(uint64(len(data))), data...)
	} else if val.CanAddr() {
		if m, ok := val.Addr().Interface().(SiaMarshaler); ok {
			data := m.MarshalSia()
			return append(EncUint64(uint64(len(data))), data...)
		}
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return []byte{0}
		}
		return append([]byte{1}, marshal(val.Elem())...)
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
		return append(EncUint64(uint64(len(s))), s...)
	case reflect.Slice:
		// slices are variable length, so prepend the length and then fallthrough to array logic
		b = EncUint64(uint64(val.Len()))
		fallthrough
	case reflect.Array:
		// special case for byte arrays
		if val.Type().Elem().Kind() == reflect.Uint8 {
			// convert array to slice so we can use Bytes()
			// can't just use Slice() because array may be unaddressable
			slice := reflect.MakeSlice(reflect.SliceOf(val.Type().Elem()), val.Len(), val.Len())
			reflect.Copy(slice, val)
			return append(b, slice.Bytes()...)
		}
		// normal slices/arrays are encoded by sequentially encoding their elements
		for i := 0; i < val.Len(); i++ {
			b = append(b, marshal(val.Index(i))...)
		}
		return
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			b = append(b, marshal(val.Field(i))...)
		}
		return
	default:
		// Marshalling should never fail. If it panics, you're doing something wrong,
		// like trying to encode a map or an unexported struct field.
		panic("could not marshal type " + val.Type().String())
	}
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
	// note that this allows us to skip any boundary checks while unmarshalling
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
	if val.CanAddr() {
		if u, ok := val.Addr().Interface().(SiaUnmarshaler); ok {
			var dataLen int
			dataLen, b = int(DecUint64(b[:8])), b[8:]
			u.UnmarshalSia(b[:dataLen])
			return dataLen + 8
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
		if b[0] == 0 {
			val.SetBool(false)
		} else if b[0] == 1 {
			val.SetBool(true)
		} else {
			panic("boolean value was not 0 or 1")
		}
		return 1
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val.SetInt(DecInt64(b[:8]))
		return 8
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val.SetUint(DecUint64(b[:8]))
		return 8
	case reflect.String:
		n := DecUint64(b[:8]) + 8
		val.SetString(string(b[8:n]))
		return int(n)
	case reflect.Slice:
		// slices are variable length, but otherwise the same as arrays.
		// just have to allocate them first, then we can fallthrough to the array logic.
		var sliceLen int
		sliceLen, b, consumed = int(DecUint64(b[:8])), b[8:], 8
		val.Set(reflect.MakeSlice(val.Type(), sliceLen, sliceLen))
		fallthrough
	case reflect.Array:
		// special case for byte arrays (e.g. hashes)
		if val.Type().Elem().Kind() == reflect.Uint8 {
			slice := reflect.ValueOf(b).Slice(0, val.Len())
			return consumed + reflect.Copy(val, slice)
		}
		// arrays are unmarshalled by sequentially unmarshalling their elements
		for i := 0; i < val.Len(); i++ {
			n := unmarshal(b, val.Index(i))
			consumed, b = consumed+n, b[n:]
		}
		return
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			n := unmarshal(b, val.Field(i))
			consumed, b = consumed+n, b[n:]
		}
		return
	default:
		panic("unknown type")
	}
}

// MarshalAll marshals all of its inputs and returns their concatenation.
func MarshalAll(v ...interface{}) (b []byte) {
	for i := range v {
		b = append(b, Marshal(v[i])...)
	}
	return
}
