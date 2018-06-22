package wallet

import (
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/siatest"
)

// walletTestDir creates a temporary testing directory for a wallet test. This
// should only every be called once per test. Otherwise it will delete the
// directory again.
func walletTestDir(t *testing.T) string {
	path := siatest.TestDir("wallet", t.Name())
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
