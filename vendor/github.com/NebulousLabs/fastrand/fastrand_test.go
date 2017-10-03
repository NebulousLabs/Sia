package fastrand

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"math"
	"math/big"
	mrand "math/rand"
	"sync"
	"testing"
	"time"
)

// panics returns true if the function fn panicked.
func panics(fn func()) (panicked bool) {
	defer func() {
		panicked = (recover() != nil)
	}()
	fn()
	return
}

// TestUint64nPanics tests that Uint64n panics if n == 0.
func TestUint64nPanics(t *testing.T) {
	// Test n = 0.
	if !panics(func() { Uint64n(0) }) {
		t.Error("expected panic for n == 0")
	}

	// Test n > 0.
	if panics(func() { Uint64n(math.MaxUint64) }) {
		t.Error("did not expect panic for n > 0")
	}
}

// TestIntnPanics tests that Intn panics if n <= 0.
func TestIntnPanics(t *testing.T) {
	// Test n < 0.
	if !panics(func() { Intn(-1) }) {
		t.Error("expected panic for n < 0")
	}

	// Test n = 0.
	if !panics(func() { Intn(0) }) {
		t.Error("expected panic for n == 0")
	}

	// Test n > 0.
	if panics(func() { Intn(1) }) {
		t.Error("did not expect panic for n > 0")
	}
}

// TestBigIntnPanics tests that BigIntn panics if n <= 0.
func TestBigIntnPanics(t *testing.T) {
	// Test n < 0.
	if !panics(func() { BigIntn(big.NewInt(-1)) }) {
		t.Error("expected panic for n < 0")
	}

	// Test n = 0.
	if !panics(func() { BigIntn(big.NewInt(0)) }) {
		t.Error("expected panic for n == 0")
	}

	// Test n > 0.
	if panics(func() { BigIntn(big.NewInt(1)) }) {
		t.Error("did not expect panic for n > 0")
	}
}

// TestUint64n tests the Uint64n function.
func TestUint64n(t *testing.T) {
	const iters = 10000
	var counts [10]uint64
	for i := 0; i < iters; i++ {
		counts[Uint64n(uint64(len(counts)))]++
	}
	exp := iters / uint64(len(counts))
	lower, upper := exp-(exp/10), exp+(exp/10)
	for i, n := range counts {
		if !(lower < n && n < upper) {
			t.Errorf("Expected range of %v-%v for index %v, got %v", lower, upper, i, n)
		}
	}
}

// TestIntn tests the Intn function.
func TestIntn(t *testing.T) {
	const iters = 10000
	var counts [10]int
	for i := 0; i < iters; i++ {
		counts[Intn(len(counts))]++
	}
	exp := iters / len(counts)
	lower, upper := exp-(exp/10), exp+(exp/10)
	for i, n := range counts {
		if !(lower < n && n < upper) {
			t.Errorf("Expected range of %v-%v for index %v, got %v", lower, upper, i, n)
		}
	}
}

// TestRead tests that Read produces output with sufficiently high entropy.
func TestRead(t *testing.T) {
	const size = 10e3

	var b bytes.Buffer
	zip, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if _, err := zip.Write(Bytes(size)); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Len() < size {
		t.Error("supposedly high entropy bytes have been compressed!")
	}
}

// TestReadConcurrent tests that concurrent calls to 'Read' will not result
// result in identical entropy being produced. Note that for this test to work,
// the points at which 'counter' and 'innerCounter' get incremented need to be
// reduced substantially, to a value like '64'. (larger than the number of
// threads, but not by much).
//
// Note that while this test is capable of catching failures, it's not
// guaranteed to.
func TestReadConcurrent(t *testing.T) {
	threads := 32

	// Spin up threads which will all be collecting entropy from 'Read' in
	// parallel.
	closeChan := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(threads)
	entropys := make([]map[string]struct{}, threads)
	for i := 0; i < threads; i++ {
		entropys[i] = make(map[string]struct{})
		go func(i int) {
			for {
				select {
				case <-closeChan:
					wg.Done()
					return
				default:
				}

				// Read 32 bytes.
				buf := make([]byte, 32)
				Read(buf)
				bufStr := string(buf)
				_, exists := entropys[i][bufStr]
				if exists {
					t.Error("got the same entropy twice out of the reader")
				}
				entropys[i][bufStr] = struct{}{}
			}
		}(i)
	}

	// Let the threads spin for a bit, then shut them down.
	time.Sleep(time.Millisecond * 1250)
	close(closeChan)
	wg.Wait()

	// Compare the entropy collected and verify that no set of 32 bytes was
	// output twice.
	allEntropy := make(map[string]struct{})
	for _, entropy := range entropys {
		for str := range entropy {
			_, exists := allEntropy[str]
			if exists {
				t.Error("got the same entropy twice out of the reader")
			}
			allEntropy[str] = struct{}{}
		}
	}
}

// TestRandConcurrent checks that there are no race conditions when using the
// rngs concurrently.
func TestRandConcurrent(t *testing.T) {
	// Spin up one goroutine for each exported function. Each goroutine calls
	// its function in a tight loop.

	funcs := []func(){
		// Read some random data into a large byte slice.
		func() { Read(make([]byte, 16e3)) },

		// Call io.Copy on the global reader.
		func() { io.CopyN(new(bytes.Buffer), Reader, 16e3) },

		// Call Intn
		func() { Intn(math.MaxUint64/4 + 1) },

		// Call BigIntn on a 256-bit int
		func() { BigIntn(new(big.Int).SetBytes(Bytes(32))) },

		// Call Perm
		func() { Perm(150) },
	}

	closeChan := make(chan struct{})
	var wg sync.WaitGroup
	for i := range funcs {
		wg.Add(1)
		go func(i int) {
			for {
				select {
				case <-closeChan:
					wg.Done()
					return
				default:
				}

				funcs[i]()
			}
		}(i)
	}

	// Allow goroutines to run for a moment.
	time.Sleep(100 * time.Millisecond)

	// Close the channel and wait for everything to clean up.
	close(closeChan)
	wg.Wait()
}

// TestPerm tests the Perm function.
func TestPerm(t *testing.T) {
	chars := "abcde" // string to be permuted
	createPerm := func() string {
		s := make([]byte, len(chars))
		for i, j := range Perm(len(chars)) {
			s[i] = chars[j]
		}
		return string(s)
	}

	// create (factorial(len(chars)) * 100) permutations
	permCount := make(map[string]int)
	for i := 0; i < 12000; i++ {
		permCount[createPerm()]++
	}

	// we should have seen each permutation approx. 100 times
	for p, n := range permCount {
		if n < 50 || n > 150 {
			t.Errorf("saw permutation %v times: %v", n, p)
		}
	}
}

// BenchmarkUint64n benchmarks the Uint64n function for small uint64s.
func BenchmarkUint64n(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Uint64n(4e3)
	}
}

// BenchmarkUint64nLarge benchmarks the Uint64n function for large uint64s.
func BenchmarkUint64nLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// constant chosen to trigger resampling (see Uint64n)
		_ = Uint64n(math.MaxUint64/2 + 1)
	}
}

// BenchmarkIntn benchmarks the Intn function for small ints.
func BenchmarkIntn(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Intn(4e3)
	}
}

// BenchmarkIntnLarge benchmarks the Intn function for large ints.
func BenchmarkIntnLarge(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// constant chosen to trigger resampling (see Intn)
		_ = Intn(math.MaxUint64/4 + 1)
	}
}

// BenchmarkBigIntn benchmarks the BigIntn function for small ints.
func BenchmarkBigIntn(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = BigIntn(big.NewInt(4e3))
	}
}

// BenchmarkBigIntnLarge benchmarks the BigIntn function for large ints.
func BenchmarkBigIntnLarge(b *testing.B) {
	// (2^63)^10
	huge := new(big.Int).Exp(big.NewInt(math.MaxInt64), big.NewInt(10), nil)
	for i := 0; i < b.N; i++ {
		_ = BigIntn(huge)
	}
}

// BenchmarkBigCryptoInt benchmarks the (crypto/rand).Int function for small ints.
func BenchmarkBigCryptoInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = rand.Int(rand.Reader, big.NewInt(4e3))
	}
}

// BenchmarkBigCryptoIntLarge benchmarks the (crypto/rand).Int function for large ints.
func BenchmarkBigCryptoIntLarge(b *testing.B) {
	// (2^63)^10
	huge := new(big.Int).Exp(big.NewInt(math.MaxInt64), big.NewInt(10), nil)
	for i := 0; i < b.N; i++ {
		_, _ = rand.Int(rand.Reader, huge)
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

// BenchmarkRead benchmarks the speed of Read for small slices.
func BenchmarkRead32(b *testing.B) {
	b.SetBytes(32)
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkRead512kb benchmarks the speed of Read for larger slices.
func BenchmarkRead512kb(b *testing.B) {
	b.SetBytes(512e3)
	buf := make([]byte, 512e3)
	for i := 0; i < b.N; i++ {
		Read(buf)
	}
}

// BenchmarkRead4Threads32 benchmarks the speed of Read when it's being using
// across four threads.
func BenchmarkRead4Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkRead4Threads512kb benchmarks the speed of Read when it's being using
// across four threads with 512kb read sizes.
func BenchmarkRead4Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkRead64Threads32 benchmarks the speed of Read when it's being using
// across 64 threads.
func BenchmarkRead64Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkRead64Threads512kb benchmarks the speed of Read when it's being using
// across 64 threads with 512kb read sizes.
func BenchmarkRead64Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
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

// BenchmarkReadCrypto512kb benchmarks the speed of (crypto/rand).Read for larger
// slices. This establishes a lower limit for BenchmarkRead512kb.
func BenchmarkReadCrypto512kb(b *testing.B) {
	b.SetBytes(512e3)
	buf := make([]byte, 512e3)
	for i := 0; i < b.N; i++ {
		rand.Read(buf)
	}
}

// BenchmarkReadCrypto4Threads32 benchmarks the speed of rand.Read when its
// being used across 4 threads with 32 byte read sizes.
func BenchmarkReadCrypto4Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				_, err := rand.Read(buf)
				if err != nil {
					b.Fatal(err)
				}
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadCrypto4Threads512kb benchmarks the speed of rand.Read when its
// being used across 4 threads with 512 kb read sizes.
func BenchmarkReadCrypto4Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				_, err := rand.Read(buf)
				if err != nil {
					b.Fatal(err)
				}
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadCrypto64Threads32 benchmarks the speed of rand.Read when its
// being used across 4 threads with 32 byte read sizes.
func BenchmarkReadCrypto64Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				_, err := rand.Read(buf)
				if err != nil {
					b.Fatal(err)
				}
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadCrypto64Threads512k benchmarks the speed of rand.Read when its
// being used across 4 threads with 512 kb read sizes.
func BenchmarkReadCrypto64Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				_, err := rand.Read(buf)
				if err != nil {
					b.Fatal(err)
				}
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadMath benchmarks the speed of (math/rand).Read for small
// slices. This establishes an upper limit for BenchmarkRead32.
func BenchmarkReadMath32(b *testing.B) {
	b.SetBytes(32)
	buf := make([]byte, 32)
	for i := 0; i < b.N; i++ {
		mrand.Read(buf)
	}
}

// BenchmarkReadMath512kb benchmarks the speed of (math/rand).Read for larger
// slices. This establishes an upper limit for BenchmarkRead512kb.
func BenchmarkReadMath512kb(b *testing.B) {
	b.SetBytes(512e3)
	buf := make([]byte, 512e3)
	for i := 0; i < b.N; i++ {
		mrand.Read(buf)
	}
}

// BenchmarkReadMath4Threads32 benchmarks the speed of ReadMath when it's being using
// across four threads.
func BenchmarkReadMath4Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				mrand.Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadMath4Threads512kb benchmarks the speed of ReadMath when it's being using
// across four threads with 512kb read sizes.
func BenchmarkReadMath4Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				mrand.Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(4 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadMath64Threads32 benchmarks the speed of ReadMath when it's being using
// across 64 threads.
func BenchmarkReadMath64Threads32(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 32)
			<-start
			for i := 0; i < b.N; i++ {
				mrand.Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 32)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}

// BenchmarkReadMath64Threads512kb benchmarks the speed of ReadMath when it's being using
// across 64 threads with 512kb read sizes.
func BenchmarkReadMath64Threads512kb(b *testing.B) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			buf := make([]byte, 512e3)
			<-start
			for i := 0; i < b.N; i++ {
				mrand.Read(buf)
			}
			wg.Done()
		}()
	}
	b.SetBytes(64 * 512e3)

	// Signal all threads to begin
	b.ResetTimer()
	close(start)
	// Wait for all threads to exit
	wg.Wait()
}
