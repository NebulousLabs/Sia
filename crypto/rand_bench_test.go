package crypto

import (
	"crypto/rand"
	"testing"
)

// BenchmarkRandIntn benchmarks the RandIntn function for large ints.
func BenchmarkRandIntn(b *testing.B) {
	for i := 0; i < b.N; i++ {
		i, err := RandIntn(4e9)
		if err != nil {
			b.Fatal(err)
		}
		if i < 0 || i >= 4e9 {
			b.Fatal("bad value for i:", i)
		}
	}
}

// BenchmarkRandIntnSmall benchmarks the RandIntn function for small ints.
func BenchmarkRandIntnSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		i, err := RandIntn(4e3)
		if err != nil {
			b.Fatal(err)
		}
		if i < 0 || i >= 4e3 {
			b.Fatal("bad value for i:", i)
		}
	}
}

// BenchmarkRead benchmarks the speed of Read for small slices.
func BenchmarkRead(b *testing.B) {
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkRead64K benchmarks the speed of Read for larger slices.
func BenchmarkRead64K(b *testing.B) {
	buf := make([]byte, 64e3)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkRandRead benchmarks the speed of rand.Read for small slices.
func BenchmarkRandRead(b *testing.B) {
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		_, err := rand.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRandRead64K benchmarks the speed of rand.Read for larger slices.
func BenchmarkRandRead64K(b *testing.B) {
	buf := make([]byte, 64e3)
	for i := 0; i < b.N; i++ {
		_, err := rand.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPerm benchmarks the speed of Perm for small slices.
func BenchmarkPerm(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := Perm(32)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPermLarge benchmarks the speed of Perm for large slices.
func BenchmarkPermLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := Perm(4e3)
		if err != nil {
			b.Fatal(err)
		}
	}
}
