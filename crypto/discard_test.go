package crypto

import (
	"bytes"
	"testing"
)

// TestUnitSecureWipe tests that the SecureWipe function sets all the elements
// in a byte slice to 0.
func TestUnitSecureWipe(t *testing.T) {
	s := []byte{1, 2, 3, 4}
	SecureWipe(s)
	if !bytes.Equal(s, make([]byte, len(s))) {
		t.Error("some bytes not set to 0")
	}
}

// TestUnitSecureWipeEdgeCases tests that SecureWipe doesn't panic on nil or
// empty slices.
func TestUnitSecureWipeEdgeCases(t *testing.T) {
	SecureWipe(nil)
	SecureWipe([]byte{})
}
