package persist

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"gitlab.com/NebulousLabs/Sia/build"
)

// TestSaveLoadJSON creates a simple object and then tries saving and loading
// it.
func TestSaveLoadJSON(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
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
	var obj2 testStruct

	// Try loading the object
	err = LoadJSON(testMeta, &obj2, obj1Filename)
	if err != nil {
		t.Fatal(err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}

	// Try loading the object using the temp file.
	err = LoadJSON(testMeta, &obj2, obj1Filename+tempSuffix)
	if err != ErrBadFilenameSuffix {
		t.Error("did not get bad filename suffix")
	}

	// Try saving the object multiple times concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 250; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				recover() // Error is irrelevant.
			}()
			SaveJSON(testMeta, obj1, obj1Filename)
		}(i)
	}
	wg.Wait()

	// Despite possible errors from saving the object many times concurrently,
	// the object should still be readable.
	err = LoadJSON(testMeta, &obj2, obj1Filename)
	if err != nil {
		t.Fatal(err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}
}

// TestLoadJSONCorruptedFiles checks that LoadJSON correctly handles various
// types of corruption that can occur during the saving process.
func TestLoadJSONCorruptedFiles(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Define the test object that will be getting loaded.
	testMeta := Metadata{"Test Struct", "v1.2.1"}
	type testStruct struct {
		One   string
		Two   uint64
		Three []byte
	}
	obj1 := testStruct{"dog", 25, []byte("more dog")}
	var obj2 testStruct

	// Try loading a file with a bad checksum.
	err := LoadJSON(testMeta, &obj2, filepath.Join("testdata", "badchecksum.json"))
	if err == nil {
		t.Error("bad checksum should have failed")
	}
	// Try loading a file where only the main has a bad checksum.
	err = LoadJSON(testMeta, &obj2, filepath.Join("testdata", "badchecksummain.json"))
	if err != nil {
		t.Error("bad checksum main failed:", err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}

	// Try loading a file with a manual checksum.
	err = LoadJSON(testMeta, &obj2, filepath.Join("testdata", "manual.json"))
	if err != nil {
		t.Error("loading file with a manual checksum should have succeeded")
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}

	// Try loading a corrupted main file.
	err = LoadJSON(testMeta, &obj2, filepath.Join("testdata", "corruptmain.json"))
	if err != nil {
		t.Error("couldn't load corrupted main:", err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}

	// Try loading a corrupted temp file.
	err = LoadJSON(testMeta, &obj2, filepath.Join("testdata", "corrupttemp.json"))
	if err != nil {
		t.Error("couldn't load corrupted main:", err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}

	// Try loading a file with no temp, and no checksum.
	err = LoadJSON(testMeta, &obj2, filepath.Join("testdata", "nochecksum.json"))
	if err != nil {
		t.Error("couldn't load no checksum:", err)
	}
	// Verify equivalence.
	if obj2.One != obj1.One {
		t.Error("persist mismatch")
	}
	if obj2.Two != obj1.Two {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, obj1.Three) {
		t.Error("persist mismatch")
	}
	if obj2.One != "dog" {
		t.Error("persist mismatch")
	}
	if obj2.Two != 25 {
		t.Error("persist mismatch")
	}
	if !bytes.Equal(obj2.Three, []byte("more dog")) {
		t.Error("persist mismatch")
	}
}

// TestSaveCorruptedMainFile checks that SaveJSON refuses to overwrite the temp
// file if the main file is corrupted.
func TestSaveJSONCorruptedMainFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create the directory used for testing.
	dir := filepath.Join(build.TempDir(persistDir), t.Name())
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Define the test object that will be getting saved.
	testMeta := Metadata{"Test Struct", "v1.2.1"}
	type testStruct struct {
		One   string
		Two   uint64
		Three []byte
	}
	// 'newObj' is different from the object that has already been saved to
	// 'corruptmain.json' and 'corruptmain.json_temp'.
	newObj := testStruct{"cat", 716, []byte("cat attack")}

	// Copy the corruptmain files to the testing dir.
	corruptMainFileSource := filepath.Join("testdata", "corruptmain.json")
	corruptMainTempFileSource := filepath.Join("testdata", "corruptmain.json_temp")
	corruptMainFileDest := filepath.Join(dir, "corruptmain.json")
	corruptMainTempFileDest := filepath.Join(dir, "corruptmain.json_temp")
	build.CopyFile(corruptMainFileSource, corruptMainFileDest)
	build.CopyFile(corruptMainTempFileSource, corruptMainTempFileDest)

	// Get all of the bytes of the temp file to verify later that the temp file
	// is unchanged after saving.
	file, err := os.Open(corruptMainTempFileDest)
	if err != nil {
		t.Fatal(err)
	}
	originalTempFileData, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	// Try saving a file when the main file has been corrupted. The save file
	// should detect that the main file has been corrupted, and it should not
	// touch the temp file. If the temp file changes, corruption has potentially
	// been introduced.
	err = SaveJSON(testMeta, newObj, corruptMainFileDest)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the temp file is untouched.
	file, err = os.Open(corruptMainTempFileDest)
	if err != nil {
		t.Fatal(err)
	}
	tempFileDataAfterBadSave, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if !bytes.Equal(tempFileDataAfterBadSave, originalTempFileData) {
		t.Error("Temp file was changed after a correupted main file save")
	}

	// Save again. This time, because the full file is correct, it should
	// overwrite the temp file.
	err = SaveJSON(testMeta, newObj, corruptMainFileDest)
	if err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(corruptMainTempFileDest)
	if err != nil {
		t.Fatal(err)
	}
	tempFileDataAfterGoodSave, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if bytes.Equal(tempFileDataAfterGoodSave, originalTempFileData) {
		t.Error("Temp file was not changed after a good save")
	}
}
