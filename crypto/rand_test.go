package crypto

import (
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

// TestPerm tests the Perm function.
func TestPerm(t *testing.T) {
	chars := "abcde" // string to be permuted
	createPerm := func() string {
		perm, err := Perm(len(chars))
		if err != nil {
			t.Fatal(err)
		}
		s := make([]byte, len(chars))
		for i, j := range perm {
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
