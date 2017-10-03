/**
 * Unit tests for Galois
 *
 * Copyright 2015, Klaus Post
 * Copyright 2015, Backblaze, Inc.
 */

package reedsolomon

import (
	"bytes"
	"testing"
)

func TestAssociativity(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		for j := 0; j < 256; j++ {
			b := byte(j)
			for k := 0; k < 256; k++ {
				c := byte(k)
				x := galAdd(a, galAdd(b, c))
				y := galAdd(galAdd(a, b), c)
				if x != y {
					t.Fatal("add does not match:", x, "!=", y)
				}
				x = galMultiply(a, galMultiply(b, c))
				y = galMultiply(galMultiply(a, b), c)
				if x != y {
					t.Fatal("multiply does not match:", x, "!=", y)
				}
			}
		}
	}
}

func TestIdentity(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		b := galAdd(a, 0)
		if a != b {
			t.Fatal("Add zero should yield same result", a, "!=", b)
		}
		b = galMultiply(a, 1)
		if a != b {
			t.Fatal("Mul by one should yield same result", a, "!=", b)
		}
	}
}

func TestInverse(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		b := galSub(0, a)
		c := galAdd(a, b)
		if c != 0 {
			t.Fatal("inverse sub/add", c, "!=", 0)
		}
		if a != 0 {
			b = galDivide(1, a)
			c = galMultiply(a, b)
			if c != 1 {
				t.Fatal("inverse div/mul", c, "!=", 1)
			}
		}
	}
}

func TestCommutativity(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		for j := 0; j < 256; j++ {
			b := byte(j)
			x := galAdd(a, b)
			y := galAdd(b, a)
			if x != y {
				t.Fatal(x, "!= ", y)
			}
			x = galMultiply(a, b)
			y = galMultiply(b, a)
			if x != y {
				t.Fatal(x, "!= ", y)
			}
		}
	}
}

func TestDistributivity(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		for j := 0; j < 256; j++ {
			b := byte(j)
			for k := 0; k < 256; k++ {
				c := byte(k)
				x := galMultiply(a, galAdd(b, c))
				y := galAdd(galMultiply(a, b), galMultiply(a, c))
				if x != y {
					t.Fatal(x, "!= ", y)
				}
			}
		}
	}
}

func TestExp(t *testing.T) {
	for i := 0; i < 256; i++ {
		a := byte(i)
		power := byte(1)
		for j := 0; j < 256; j++ {
			x := galExp(a, j)
			if x != power {
				t.Fatal(x, "!=", power)
			}
			power = galMultiply(power, a)
		}
	}
}

func TestGalois(t *testing.T) {
	// These values were copied output of the Python code.
	if galMultiply(3, 4) != 12 {
		t.Fatal("galMultiply(3, 4) != 12")
	}
	if galMultiply(7, 7) != 21 {
		t.Fatal("galMultiply(7, 7) != 21")
	}
	if galMultiply(23, 45) != 41 {
		t.Fatal("galMultiply(23, 45) != 41")
	}

	// Test slices (>32 entries to test assembler -- AVX2 & NEON)
	in := []byte{0, 1, 2, 3, 4, 5, 6, 10, 50, 100, 150, 174, 201, 255, 99, 32, 67, 85, 200, 199, 198, 197, 196, 195, 194, 193, 192, 191, 190, 189, 188, 187, 186, 185}
	out := make([]byte, len(in))
	galMulSlice(25, in, out, false, false)
	expect := []byte{0x0, 0x19, 0x32, 0x2b, 0x64, 0x7d, 0x56, 0xfa, 0xb8, 0x6d, 0xc7, 0x85, 0xc3, 0x1f, 0x22, 0x7, 0x25, 0xfe, 0xda, 0x5d, 0x44, 0x6f, 0x76, 0x39, 0x20, 0xb, 0x12, 0x11, 0x8, 0x23, 0x3a, 0x75, 0x6c, 0x47}
	if 0 != bytes.Compare(out, expect) {
		t.Errorf("got %#v, expected %#v", out, expect)
	}
	expectXor := []byte{0x0, 0x2d, 0x5a, 0x77, 0xb4, 0x99, 0xee, 0x2f, 0x79, 0xf2, 0x7, 0x51, 0xd4, 0x19, 0x31, 0xc9, 0xf8, 0xfc, 0xf9, 0x4f, 0x62, 0x15, 0x38, 0xfb, 0xd6, 0xa1, 0x8c, 0x96, 0xbb, 0xcc, 0xe1, 0x22, 0xf, 0x78}
	galMulSliceXor(52, in, out, false, false)
	if 0 != bytes.Compare(out, expectXor) {
		t.Errorf("got %#v, expected %#v", out, expectXor)
	}

	galMulSlice(177, in, out, false, false)
	expect = []byte{0x0, 0xb1, 0x7f, 0xce, 0xfe, 0x4f, 0x81, 0x9e, 0x3, 0x6, 0xe8, 0x75, 0xbd, 0x40, 0x36, 0xa3, 0x95, 0xcb, 0xc, 0xdd, 0x6c, 0xa2, 0x13, 0x23, 0x92, 0x5c, 0xed, 0x1b, 0xaa, 0x64, 0xd5, 0xe5, 0x54, 0x9a}
	if 0 != bytes.Compare(out, expect) {
		t.Errorf("got %#v, expected %#v", out, expect)
	}

	expectXor = []byte{0x0, 0xc4, 0x95, 0x51, 0x37, 0xf3, 0xa2, 0xfb, 0xec, 0xc5, 0xd0, 0xc7, 0x53, 0x88, 0xa3, 0xa5, 0x6, 0x78, 0x97, 0x9f, 0x5b, 0xa, 0xce, 0xa8, 0x6c, 0x3d, 0xf9, 0xdf, 0x1b, 0x4a, 0x8e, 0xe8, 0x2c, 0x7d}
	galMulSliceXor(117, in, out, false, false)
	if 0 != bytes.Compare(out, expectXor) {
		t.Errorf("got %#v, expected %#v", out, expectXor)
	}

	if galExp(2, 2) != 4 {
		t.Fatal("galExp(2, 2) != 4")
	}
	if galExp(5, 20) != 235 {
		t.Fatal("galExp(5, 20) != 235")
	}
	if galExp(13, 7) != 43 {
		t.Fatal("galExp(13, 7) != 43")
	}
}

func TestSliceGalADD(t *testing.T) {

	lengthList := []int{16, 32, 34}
	for _, length := range lengthList {
		in := make([]byte, length)
		fillRandom(in)
		out := make([]byte, length)
		fillRandom(out)
		expect := make([]byte, length)
		for i := range expect {
			expect[i] = in[i] ^ out[i]
		}
		sliceXor(in, out, false)
		if 0 != bytes.Compare(out, expect) {
			t.Errorf("got %#v, expected %#v", out, expect)
		}
		fillRandom(out)
		for i := range expect {
			expect[i] = in[i] ^ out[i]
		}
		sliceXor(in, out, true)
		if 0 != bytes.Compare(out, expect) {
			t.Errorf("got %#v, expected %#v", out, expect)
		}
	}

	for i := 0; i < 256; i++ {
		a := byte(i)
		for j := 0; j < 256; j++ {
			b := byte(j)
			for k := 0; k < 256; k++ {
				c := byte(k)
				x := galAdd(a, galAdd(b, c))
				y := galAdd(galAdd(a, b), c)
				if x != y {
					t.Fatal("add does not match:", x, "!=", y)
				}
				x = galMultiply(a, galMultiply(b, c))
				y = galMultiply(galMultiply(a, b), c)
				if x != y {
					t.Fatal("multiply does not match:", x, "!=", y)
				}
			}
		}
	}
}

func benchmarkGalois(b *testing.B, size int) {
	in := make([]byte, size)
	out := make([]byte, size)

	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		galMulSlice(25, in[:], out[:], true, false)
	}
}

func BenchmarkGalois128K(b *testing.B) {
	benchmarkGalois(b, 128*1024)
}

func BenchmarkGalois1M(b *testing.B) {
	benchmarkGalois(b, 1024*1024)
}

func benchmarkGaloisXor(b *testing.B, size int) {
	in := make([]byte, size)
	out := make([]byte, size)

	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		galMulSliceXor(177, in[:], out[:], true, false)
	}
}

func BenchmarkGaloisXor128K(b *testing.B) {
	benchmarkGaloisXor(b, 128*1024)
}

func BenchmarkGaloisXor1M(b *testing.B) {
	benchmarkGaloisXor(b, 1024*1024)
}
