// Package encoding converts arbitrary objects into byte slices, and vis
// versa. It also contains helper functions for reading and writing length-
// prefixed data. The encoding rules are as follows:
//
// Objects are encoded as binary data, without type information. The receiver
// uses context to determine the type to decode into.
//
// Integers are little-endian, and are always encoded as 8 bytes, i.e. their
// int64 or uint64 equivalent.
//
// Booleans are encoded as one byte, either zero (false) or one (true). No
// other values may be used.
//
// Nil pointers are equivalent to "false," i.e. a single zero byte. Valid
// pointers are represented by a "true" byte (0x01) followed by the encoding
// of the dereferenced value.
//
// Variable-length types, such as strings and slices, are represented by an 8-byte
// length-prefix followed by the encoded value.
//
// Slices and structs are simply the concatenation of their encoded elements.
// Byte slices are not subject to the 8-byte integer rule; they are encoded as
// their literal representation, one byte per byte.
//
// The ordering of struct fields is determined by their type definition. For
// example:
//
//   type foo struct { S string; I int }
//
//   Marshal(foo{"bar", 3}) == append(Marshal("bar"), Marshal(3)...)
//
// Finally, if a type implements the SiaMarshaler interface, its MarshalSia
// method will be used to encode the type. The resulting byte slice will be
// length-prefixed like any other variable-length type. During decoding, the
// type is decoded as a byte slice, and then passed to UnmarshalSia.
package encoding

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
)

// A SiaMarshaler can encode itself as a byte slice.
type SiaMarshaler interface {
	MarshalSia() []byte
}

// A SiaUnmarshaler can decode itself from a byte slice. If a decoding error
// occurs, UnmarshalSia should panic.
type SiaUnmarshaler interface {
	UnmarshalSia([]byte)
}

// An Encoder writes objects to an output stream.
type Encoder struct {
	w io.Writer
}

// Encode writes the encoding of v to the stream. For encoding details, see
// the package docstring.
func (e *Encoder) Encode(v interface{}) error {
	return e.encode(reflect.ValueOf(v))
}

func (e *Encoder) encode(val reflect.Value) error {
	// check for MarshalSia interface first
	if m, ok := val.Interface().(SiaMarshaler); ok {
		return WritePrefix(e.w, m.MarshalSia())
	} else if val.CanAddr() {
		if m, ok := val.Addr().Interface().(SiaMarshaler); ok {
			return WritePrefix(e.w, m.MarshalSia())
		}
	}

	switch val.Kind() {
	case reflect.Ptr:
		// write either a 1 or 0
		if err := e.Encode(!val.IsNil()); err != nil {
			return err
		}
		if !val.IsNil() {
			return e.encode(val.Elem())
		}
	case reflect.Bool:
		if val.Bool() {
			_, err := e.w.Write([]byte{1})
			return err
		} else {
			_, err := e.w.Write([]byte{0})
			return err
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := e.w.Write(EncInt64(val.Int()))
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := e.w.Write(EncUint64(val.Uint()))
		return err
	case reflect.String:
		return WritePrefix(e.w, []byte(val.String()))
	case reflect.Slice:
		// slices are variable length, so prepend the length and then fallthrough to array logic
		if _, err := e.w.Write(EncUint64(uint64(val.Len()))); err != nil {
			return err
		}
		fallthrough
	case reflect.Array:
		// special case for byte arrays
		if val.Type().Elem().Kind() == reflect.Uint8 {
			// convert array to slice so we can use Bytes()
			// can't just use Slice() because array may be unaddressable
			slice := reflect.MakeSlice(reflect.SliceOf(val.Type().Elem()), val.Len(), val.Len())
			reflect.Copy(slice, val)
			_, err := e.w.Write(slice.Bytes())
			return err
		}
		// normal slices/arrays are encoded by sequentially encoding their elements
		for i := 0; i < val.Len(); i++ {
			if err := e.encode(val.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			if err := e.encode(val.Field(i)); err != nil {
				return err
			}
		}
	default:
		// Marshalling should never fail. If it panics, you're doing something wrong,
		// like trying to encode a map or an unexported struct field.
		panic("could not marshal type " + val.Type().String())
	}
	return nil
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w}
}

// Marshal returns the encoding of v. For encoding details, see the package
// docstring.
func Marshal(v interface{}) []byte {
	b := new(bytes.Buffer)
	NewEncoder(b).Encode(v) // no error possible when using a bytes.Buffer
	return b.Bytes()
}

// MarshalAll encodes all of its inputs and returns their concatenation.
func MarshalAll(v ...interface{}) []byte {
	b := new(bytes.Buffer)
	enc := NewEncoder(b)
	for i := range v {
		enc.Encode(v[i]) // no error possible when using a bytes.Buffer
	}
	return b.Bytes()
}

// WriteFile writes v to a file. The file will be created if it does not exist.
func WriteFile(filename string, v interface{}) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	return NewEncoder(file).Encode(v)
}

// A Decoder reads and decodes values from an input stream.
type Decoder struct {
	r io.Reader
}

// Decode reads the next encoded value from its input stream and stores it in
// v, which must be a pointer. The decoding rules are the inverse of those
// specified in the package docstring.
func (d *Decoder) Decode(v interface{}) (err error) {
	// v must be a pointer
	pval := reflect.ValueOf(v)
	if pval.Kind() != reflect.Ptr || pval.IsNil() {
		return errors.New("must pass a valid pointer to Decode")
	}

	// catch decoding panics and convert them to errors
	// note that this allows us to skip boundary checks during decoding
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("could not decode type %s: %v", pval.Elem().Type().String(), r)
		}
	}()

	d.decode(pval.Elem())
	return
}

func (d *Decoder) readN(n int) []byte {
	b := make([]byte, n)
	_, err := io.ReadFull(d.r, b)
	if err != nil {
		panic(err)
	}
	return b
}

func (d *Decoder) readPrefix() []byte {
	// TODO: what should maxlen be?
	b, err := ReadPrefix(d.r, 1<<32)
	if err != nil {
		panic(err)
	}
	return b
}

func (d *Decoder) decode(val reflect.Value) {
	// check for UnmarshalSia interface first
	if val.CanAddr() {
		if u, ok := val.Addr().Interface().(SiaUnmarshaler); ok {
			u.UnmarshalSia(d.readPrefix())
			return
		}
	}

	switch val.Kind() {
	case reflect.Ptr:
		var valid bool
		d.decode(reflect.ValueOf(&valid).Elem())
		// nil pointer, nothing to decode
		if !valid {
			return
		}
		// make sure we aren't decoding into nil
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}
		d.decode(val.Elem())
	case reflect.Bool:
		b := d.readN(1)
		if b[0] > 1 {
			panic("boolean value was not 0 or 1")
		}
		val.SetBool(b[0] == 1)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val.SetInt(DecInt64(d.readN(8)))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val.SetUint(DecUint64(d.readN(8)))
	case reflect.String:
		val.SetString(string(d.readPrefix()))
	case reflect.Slice:
		// slices are variable length, but otherwise the same as arrays.
		// just have to allocate them first, then we can fallthrough to the array logic.
		sliceLen := int(DecUint64(d.readN(8)))
		val.Set(reflect.MakeSlice(val.Type(), sliceLen, sliceLen))
		fallthrough
	case reflect.Array:
		// special case for byte arrays (e.g. hashes)
		if val.Type().Elem().Kind() == reflect.Uint8 {
			// convert val to a slice and read into it directly
			b := val.Slice(0, val.Len())
			_, err := io.ReadFull(d.r, b.Bytes())
			if err != nil {
				panic(err)
			}
			return
		}
		// arrays are unmarshalled by sequentially unmarshalling their elements
		for i := 0; i < val.Len(); i++ {
			d.decode(val.Index(i))
		}
		return
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			d.decode(val.Field(i))
		}
		return
	default:
		panic("unknown type")
	}
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r}
}

// Unmarshal decodes the encoded value b and stores it in v, which must be a
// pointer. The decoding rules are the inverse of those specified in the
// package docstring.
func Unmarshal(b []byte, v interface{}) error {
	r := bytes.NewReader(b)
	return NewDecoder(r).Decode(v)
}

// ReadFile reads the contents of a file and decodes them into v.
func ReadFile(filename string, v interface{}) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	return NewDecoder(file).Decode(v)
}
