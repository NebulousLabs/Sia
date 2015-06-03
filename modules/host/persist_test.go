package host

import (
	"testing"
)

// TestSaveLoad tests that saving and loading a Host restores its data.
func TestSaveLoad(t *testing.T) {
	ht := CreateHostTester("TestSaveLoad", t)
	ht.testAllocation()
	err := ht.host.save()
	if err != nil {
		ht.t.Fatal(err)
	}
	err = ht.host.load()
	if err != nil {
		ht.t.Fatal(err)
	}
}
