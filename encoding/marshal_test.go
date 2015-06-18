package encoding

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// dummy types to test encoding
type (
	// basic
	test0 struct {
		B bool
		I int32
		U uint16
		S string
	}
	// slice/array
	test1 struct {
		Is []int32
		Bs []byte
		Sa [3]string
		Ba [3]byte
	}
	// nested
	test2 struct {
		T test0
	}
	// embedded
	test3 struct {
		test2
	}
	// pointer
	test4 struct {
		P *test0
	}
	// private field -- need to implement MarshalSia/UnmarshalSia
	test5 struct {
		s string
	}
	// private field with pointer receiver
	test6 struct {
		s string
	}
)

func (t test5) MarshalSia() []byte { return []byte(t.s) }

func (t *test5) UnmarshalSia(b []byte) { t.s = string(b) }

func (t *test6) MarshalSia() []byte { return []byte(t.s) }

func (t *test6) UnmarshalSia(b []byte) { t.s = string(b) }

var testStructs = []interface{}{
	test0{false, 65537, 256, "foo"},
	test1{[]int32{1, 2, 3}, []byte("foo"), [3]string{"foo", "bar", "baz"}, [3]byte{'f', 'o', 'o'}},
	test2{test0{false, 65537, 256, "foo"}},
	test3{test2{test0{false, 65537, 256, "foo"}}},
	test4{&test0{false, 65537, 256, "foo"}},
	test5{"foo"},
	&test6{"foo"},
}

var testEncodings = [][]byte{
	{0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
	{3, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0,
		0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o', 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o', 3,
		0, 0, 0, 0, 0, 0, 0, 'b', 'a', 'r', 3, 0, 0, 0, 0, 0, 0, 0, 'b', 'a', 'z', 'f', 'o', 'o'},
	{0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
	{0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
	{1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
	{3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
	{3, 0, 0, 0, 0, 0, 0, 0, 'f', 'o', 'o'},
}

func TestEncode(t *testing.T) {
	// use Marshal for convenience
	for i := range testStructs {
		b := Marshal(testStructs[i])
		if bytes.Compare(b, testEncodings[i]) != 0 {
			t.Errorf("bad encoding of testStructs[%d]: \nexp:\t%v\ngot:\t%v", i, testEncodings[i], b)
		}
	}

	// badWriter should fail on every encode
	enc := NewEncoder(new(badWriter))
	for i := range testStructs {
		err := enc.Encode(testStructs[i])
		if err != io.ErrShortWrite {
			t.Error("expected ErrShortWrite, got", err)
		}
	}
	// special case, not covered by testStructs
	err := enc.Encode(struct{ U [3]uint16 }{[3]uint16{1, 2, 3}})
	if err != io.ErrShortWrite {
		t.Error("expected ErrShortWrite, got", err)
	}

	// bad type
	defer func() {
		if recover() == nil {
			t.Error("expected panic, got nil")
		}
	}()
	enc.Encode(map[int]int{})
}

func TestDecode(t *testing.T) {
	// use Unmarshal for convenience
	var emptyStructs = []interface{}{&test0{}, &test1{}, &test2{}, &test3{}, &test4{}, &test5{}, &test6{}}
	for i := range testEncodings {
		err := Unmarshal(testEncodings[i], emptyStructs[i])
		if err != nil {
			t.Error(err)
		}
	}

	// bad boolean
	err := Unmarshal([]byte{3}, new(bool))
	if err == nil || err.Error() != "could not decode type bool: boolean value was not 0 or 1" {
		t.Error("expected bool error, got", err)
	}

	// non-pointer
	err = Unmarshal([]byte{1, 2, 3}, "foo")
	if err != errBadPointer {
		t.Error("expected errBadPointer, got", err)
	}

	// unknown type
	err = Unmarshal([]byte{1, 2, 3}, new(map[int]int))
	if err == nil || err.Error() != "could not decode type map[int]int: unknown type" {
		t.Error("expected unknown type error, got", err)
	}

	// big slice (larger than maxSliceLen)
	err = Unmarshal(EncUint64(maxSliceLen+1), new([]byte))
	if err == nil || err.Error() != "could not decode type []uint8: slice is too large" {
		t.Error("expected large slice error, got", err)
	}

	// massive slice (larger than MaxInt32)
	err = Unmarshal(EncUint64(1<<32), new([]byte))
	if err == nil || err.Error() != "could not decode type []uint8: slice is too large" {
		t.Error("expected large slice error, got", err)
	}

	// many small slices (total larger than maxDecodeLen)
	bigSlice := strings.Split(strings.Repeat("0123456789abcdefghijklmnopqrstuvwxyz", (maxSliceLen/16)-1), "0")
	err = Unmarshal(Marshal(bigSlice), new([]string))
	if err == nil || err.Error() != "could not decode type []string: encoded type exceeds size limit" {
		t.Error("expected large slice error, got", err)
	}

	// badReader should fail on every decode
	dec := NewDecoder(new(badReader))
	for i := range testEncodings {
		err := dec.Decode(emptyStructs[i])
		if err == nil {
			t.Error("expected error, got nil")
		}
	}
	// special case, not covered by testStructs
	err = dec.Decode(new([3]byte))
	if err == nil || err.Error() != "could not decode type [3]uint8: EOF" {
		t.Error("expected EOF error, got", err)
	}

}

func TestMarshalUnmarshal(t *testing.T) {
	var emptyStructs = []interface{}{&test0{}, &test1{}, &test2{}, &test3{}, &test4{}, &test5{}, &test6{}}
	for i := range testStructs {
		b := Marshal(testStructs[i])
		err := Unmarshal(b, emptyStructs[i])
		if err != nil {
			t.Error(err)
		}
	}
}

func TestEncodeDecode(t *testing.T) {
	var emptyStructs = []interface{}{&test0{}, &test1{}, &test2{}, &test3{}, &test4{}, &test5{}, &test6{}}
	b := new(bytes.Buffer)
	enc := NewEncoder(b)
	dec := NewDecoder(b)
	for i := range testStructs {
		enc.Encode(testStructs[i])
		err := dec.Decode(emptyStructs[i])
		if err != nil {
			t.Error(err)
		}
	}
}

func TestMarshalAll(t *testing.T) {
	var b []byte
	for i := range testStructs {
		b = append(b, Marshal(testStructs[i])...)
	}

	expected := MarshalAll(testStructs...)
	if bytes.Compare(b, expected) != 0 {
		t.Errorf("expected %v, got %v", expected, b)
	}
}

func TestReadWriteFile(t *testing.T) {
	// standard
	os.MkdirAll(build.TempDir("encoding"), 0777)
	path := build.TempDir("encoding", "TestReadWriteFile")
	err := WriteFile(path, testStructs[3])
	if err != nil {
		t.Fatal(err)
	}

	var obj test4
	err = ReadFile(path, &obj)
	if err != nil {
		t.Error(err)
	}

	// bad paths
	err = WriteFile("/foo/bar", "baz")
	if err == nil {
		t.Error("expected error, got nil")
	}
	err = ReadFile("/foo/bar", nil)
	if err == nil {
		t.Error("expected error, got nil")
	}
}
