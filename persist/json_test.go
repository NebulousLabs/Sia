package persist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestSaveLoadJSON creates a simple object and then tries saving and loading
// it.
func TestSaveLoadJSON(t *testing.T) {
	// Create the directory used for testing.
	dir := filepath.Join(build.TempDir(persistDir), t.Name())
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Create and save the test object.
	testMeta := Metadata{"Test Struct", "v1.2.1"}
	type testStruct struct {
		One   string
		Two   uint64
		Three []byte
	}

	obj1 := testStruct{"dog", 25, []byte("more dog")}
	obj1Filename := filepath.Join(dir, "obj1.json")
	err = SaveJSON(testMeta, obj1, obj1Filename)
	if err != nil {
		t.Fatal(err)
	}

	// Try loading the object
	var obj2 testStruct
	err = LoadJSON(testMeta, &obj2, obj1Filename)
	if err != nil {
		t.Fatal(err)
	}
}
