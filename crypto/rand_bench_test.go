package crypto

import (
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
