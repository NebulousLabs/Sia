package encoding

import (
	"bytes"
	"testing"
)

// dummy types to test encoding
type (
	// basic
	test0 struct {
		I int32
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
)

// here we use a single length byte, unlike the standard marshalling scheme
func (t test5) MarshalSia() []byte {
	return []byte(t.s)
}

func (t *test5) UnmarshalSia(b []byte) {
	t.s = string(b)
}

var testStructs = []interface{}{
	test0{65537, "foo"},
	test1{[]int32{1, 2, 3}, []byte("foo"), [3]string{"foo", "bar", "baz"}, [3]byte{'f', 'o', 'o'}},
	test2{test0{65537, "foo"}},
	test3{test2{test0{65537, "foo"}}},
	test4{&test0{65537, "foo"}},
	test5{"foo"},
}

func TestMarshaling(t *testing.T) {
	var emptyStructs = []interface{}{&test0{}, &test1{}, &test2{}, &test3{}, &test4{}, &test5{}}

	for i := range testStructs {
		b := Marshal(testStructs[i])
		err := Unmarshal(b, emptyStructs[i])
		if err != nil {
			t.Error(err)
		}
		//t.Log("\n", testStructs[i], "\n", emptyStructs[i])
	}
}

func TestEncoding(t *testing.T) {
	var emptyStructs = []interface{}{&test0{}, &test1{}, &test2{}, &test3{}, &test4{}, &test5{}}

	b := new(bytes.Buffer)
	enc := NewEncoder(b)
	dec := NewDecoder(b)
	for i := range testStructs {
		enc.Encode(testStructs[i])
		err := dec.Decode(emptyStructs[i])
		if err != nil {
			t.Error(err)
		}
		//t.Log("\n", testStructs[i], "\n", emptyStructs[i])
	}
}
