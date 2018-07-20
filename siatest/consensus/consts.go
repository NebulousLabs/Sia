package consensus

import (
	"os"

	"gitlab.com/NebulousLabs/Sia/siatest"
)

// consensusTestDir creates a temporary testing directory for a consensus. This
// should only every be called once per test. Otherwise it will delete the
// directory again.
func consensusTestDir(testName string) string {
	path := siatest.TestDir("consensus", testName)
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
