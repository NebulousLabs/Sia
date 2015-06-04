package persist

import (
	"bytes"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

func TestSaveLoad(t *testing.T) {
	var meta = Metadata{"TestSaveLoad", "0.1"}
	var saveData int = 3
	buf := new(bytes.Buffer)

	// save data to buffer
	err := Save(meta, saveData, buf)
	if err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()

	// load valid data
	var loadData int
	err = Load(meta, &loadData, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if loadData != saveData {
		t.Fatalf("loaded data (%v) does not match saved data (%v)", loadData, saveData)
	}

	// load with bad metadata
	err = Load(Metadata{"BadTestSaveLoad", "0.1"}, &loadData, bytes.NewReader(data))
	if err != ErrBadHeader {
		t.Fatal("expected ErrBadHeader, got", err)
	}
	err = Load(Metadata{"TestSaveLoad", "-1"}, &loadData, bytes.NewReader(data))
	if err != ErrBadVersion {
		t.Fatal("expected ErrBadVersion, got", err)
	}

	// corrupt data, moving back to front
	data[21] = '}'
	err = Load(meta, &loadData, bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error when loading corrupted data")
	}
	data[14] = '}'
	err = Load(meta, &loadData, bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error when loading corrupted data")
	}
	data[0] = '}'
	err = Load(meta, &loadData, bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error when loading corrupted data")
	}
}

func TestSaveLoadFile(t *testing.T) {
	var meta = Metadata{"TestSaveLoadFile", "0.1"}
	var saveData int = 3

	filename := build.TempDir("TestSaveLoadFile")
	err := SaveFile(meta, saveData, filename)
	if err != nil {
		t.Fatal(err)
	}

	var loadData int
	err = LoadFile(meta, &loadData, filename)
	if err != nil {
		t.Fatal(err)
	}
	if loadData != saveData {
		t.Fatalf("loaded data (%v) does not match saved data (%v)", loadData, saveData)
	}
}
