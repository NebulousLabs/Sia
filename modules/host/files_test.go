package host

import (
	"testing"
)

// TestSaveLoad tests that saving and loading a Host restores its data.
// TODO: expand this once Host testing is fleshed out.
func TestSaveLoad(t *testing.T) {
	ht := CreateHostTester(t)
	err := ht.save("../../hostdata_test")
	if err != nil {
		ht.Fatal(err)
	}
	err = ht.load("../../hostdata_test")
	if err != nil {
		ht.Fatal(err)
	}
}
