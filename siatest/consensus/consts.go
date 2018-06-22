package consensus

import (
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// consensusTestDir creates a temporary testing directory for a consensus. This
// should only every be called once per test. Otherwise it will delete the
// directory again.
func consensusTestDir(t *testing.T) string {
	path := siatest.TestDir("consensus", t.Name())
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
