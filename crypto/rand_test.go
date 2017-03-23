package crypto

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// panics returns true if the function fn panicked.
func panics(fn func()) (panicked bool) {
	defer func() {
		panicked = (recover() != nil)
	}()
	fn()
	return
}

// TestRandIntnPanics tests that RandIntn panics if n <= 0.
func TestRandIntnPanics(t *testing.T) {
	// Test n = 0.
	if !panics(func() { RandIntn(0) }) {
		t.Error("expected panic for n <= 0")
	}

	// Test n < 0.
	if !panics(func() { RandIntn(-1) }) {
		t.Error("expected panic for n <= 0")
	}
}

// TestRandIntn tests the RandIntn function.
func TestRandIntn(t *testing.T) {
	const iters = 10000
	var counts [10]int
	for i := 0; i < iters; i++ {
		counts[RandIntn(len(counts))]++
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
	if _, err := zip.Write(RandBytes(size)); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Len() < size {
		t.Error("supposedly high entropy bytes have been compressed!")
	}
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
