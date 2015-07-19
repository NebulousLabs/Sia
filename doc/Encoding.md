Encoding
========

The encoding package converts arbitrary objects into byte slices, and vice
versa. Objects are encoded as binary data, without type information. The
decoder will attempt to decode its input bytes into whatever type it is
passed. For example:

```go
Marshal(int64(3))  ==  []byte{3, 0, 0, 0, 0, 0, 0, 0}

var x int64
Unmarshal([]byte{3, 0, 0, 0, 0, 0, 0, 0}, &x)
// x == 3
```

Note that this leads to some ambiguity. Since an `int64` and a `uint64` are
both 8 bytes long, it is possible to encode an `int64` and succesfully decode
it as a `uint64`. As a result, it is imperative that *the decoder knows
exactly what it is decoding*. Developers must rely on context to determine
what type to decode into.

The specific rules for encoding Go's builtin types are as follows:

Integers are little-endian, and are always encoded as 8 bytes, i.e. their
`int64` or `uint64` equivalent.

Booleans are encoded as one byte, either zero (false) or one (true). No other
values may be used.

Nil pointers are equivalent to "false," i.e. a single zero byte. Valid
pointers are represented by a "true" byte (0x01) followed by the encoding of
the dereferenced value.

Variable-length types, such as strings and slices, are represented by an
8-byte unsigned length prefix followed by the encoded value. Strings are
encoded as their literal UTF-8 bytes. Slices are encoded as the concatenation
of their encoded elements. For example:

```go
//                                  slice len: 1     string len: 3   string data
Marshal([]string{"foo"}) == []byte{1,0,0,0,0,0,0,0, 3,0,0,0,0,0,0,0, 'f','o','o'}
```

Maps are not supported; attempting to encode a map will cause `Marshal` to
panic. This is because their elements are not ordered in a consistent way, and
it is imperative that this encoding scheme be deterministic. To encode a map,
either convert it to a slice of structs, or define a `MarshalSia` method (see
below).

Arrays and structs are simply the concatenation of their encoded elements.
Byte slices are not subject to the 8-byte integer rule; they are encoded as
their literal representation, one byte per byte.

All struct fields must be exported. (For some types this is a bit awkward, so
this rule is subject to change.) The ordering of struct fields is determined
by their type definition. For example:

```go
type foo struct {
	S string
	I int
}

Marshal(foo{"bar", 3}) == append(Marshal("bar"), Marshal(3)...)
```

Finally, if a type implements the SiaMarshaler interface, its MarshalSia
method will be used to encode the type. The resulting byte slice will be
length-prefixed like any other variable-length type. During decoding, the type
is decoded as a byte slice, and then passed to UnmarshalSia. The most
prevalent example of this is the `Currency` type, which is encoded as a
variable-length big-endian unsigned integer.
