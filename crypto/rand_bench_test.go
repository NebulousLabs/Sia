package crypto

import (
	"crypto/rand"
	"math"
	"testing"
	"time"
)

// BenchmarkRandIntn benchmarks the RandIntn function for small ints.
func BenchmarkRandIntn(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = RandIntn(4e3)
	}
}

// BenchmarkRandIntnLarge benchmarks the RandIntn function for large ints.
func BenchmarkRandIntnLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// constant chosen to trigger resampling (see RandIntn)
		_ = RandIntn(math.MaxUint64/4 + 1)
	}
}

// BenchmarkRead benchmarks the speed of Read for small slices.
func BenchmarkRead32(b *testing.B) {
	b.SetBytes(32)
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkRead64K benchmarks the speed of Read for larger slices.
func BenchmarkRead64K(b *testing.B) {
	b.SetBytes(64e3)
	buf := make([]byte, 64e3)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkReadContention benchmarks the speed of Read when 4 other
// goroutines are calling RandIntn in a tight loop.
func BenchmarkReadContention(b *testing.B) {
	b.SetBytes(32)
	for j := 0; j < 4; j++ {
		go func() {
			for {
				RandIntn(1)
				time.Sleep(time.Microsecond)
			}
		}()
	}
	buf := make([]byte, 32)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkReadCrypto benchmarks the speed of (crypto/rand).Read for small
// slices. This establishes a lower limit for BenchmarkRead32.
func BenchmarkReadCrypto32(b *testing.B) {
	b.SetBytes(32)
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		rand.Read(buf)
	}
}

// BenchmarkReadCrypto64K benchmarks the speed of (crypto/rand).Read for larger
// slices. This establishes a lower limit for BenchmarkRead64K.
func BenchmarkReadCrypto64K(b *testing.B) {
	b.SetBytes(64e3)
	buf := make([]byte, 64e3)
	for i := 0; i < b.N; i++ {
		rand.Read(buf)
	}
}

// BenchmarkPerm benchmarks the speed of Perm for small slices.
func BenchmarkPerm32(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Perm(32)
	}
}

// BenchmarkPermLarge benchmarks the speed of Perm for large slices.
func BenchmarkPermLarge4k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Perm(4e3)
	}
}
