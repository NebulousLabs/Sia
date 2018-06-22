package renter

import (
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// renterTestDir creates a temporary testing directory for a renter test. This
// should only every be called once per test. Otherwise it will delete the
// directory again.
func renterTestDir(t *testing.T) string {
	path := siatest.TestDir("renter", t.Name())
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
