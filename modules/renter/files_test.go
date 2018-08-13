package renter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/errors"
)

// TestFileNumChunks checks the numChunks method of the file type.
func TestFileNumChunks(t *testing.T) {
	tests := []struct {
		size           uint64
		pieceSize      uint64
		piecesPerChunk int
		expNumChunks   uint64
	}{
		{100, 10, 1, 10}, // evenly divides
		{100, 10, 2, 5},  // evenly divides

		{101, 10, 1, 11}, // padded
		{101, 10, 2, 6},  // padded

		{10, 100, 1, 1}, // larger piece than file
		{0, 10, 1, 1},   // 0-length
	}

	for _, test := range tests {
		rsc, _ := NewRSCode(test.piecesPerChunk, 1) // can't use 0
		f := newFile(t.Name(), rsc, test.pieceSize, test.size, 0777, "")
		if f.NumChunks() != test.expNumChunks {
			t.Errorf("Test %v: expected %v, got %v", test, test.expNumChunks, f.NumChunks())
		}
	}
}

// TestFileAvailable probes the available method of the file type.
func TestFileAvailable(t *testing.T) {
	rsc, _ := NewRSCode(1, 1) // can't use 0
	f := newFile(t.Name(), rsc, pieceSize, 100, 0777, "")
	neverOffline := make(map[string]bool)

	if f.Available(neverOffline) {
		t.Error("file should not be available")
	}

	for i := uint64(0); i < f.NumChunks(); i++ {
		f.AddPiece(types.SiaPublicKey{}, i, 0, crypto.Hash{})
	}

	if !f.Available(neverOffline) {
		t.Error("file should be available")
	}

	specificOffline := make(map[string]bool)
	specificOffline[string(types.SiaPublicKey{}.Key)] = true
	if f.Available(specificOffline) {
		t.Error("file should not be available")
	}
}

// TestFileUploadedBytes tests that uploadedBytes() returns a value equal to
// the number of sectors stored via contract times the size of each sector.
func TestFileUploadedBytes(t *testing.T) {
	// ensure that a piece fits within a sector
	rsc, _ := NewRSCode(1, 3)
	f := newFile(t.Name(), rsc, modules.SectorSize/2, 1000, 0777, "")
	for i := uint64(0); i < 4; i++ {
		err := f.AddPiece(types.SiaPublicKey{}, uint64(0), i, crypto.Hash{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.UploadedBytes() != 4*modules.SectorSize {
		t.Errorf("expected uploadedBytes to be 8, got %v", f.UploadedBytes())
	}
}

// TestFileUploadProgressPinning verifies that uploadProgress() returns at most
// 100%, even if more pieces have been uploaded,
func TestFileUploadProgressPinning(t *testing.T) {
	rsc, _ := NewRSCode(1, 1)
	f := newFile(t.Name(), rsc, 2, 4, 0777, "")
	for i := uint64(0); i < 2; i++ {
		err1 := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(0)}}, uint64(0), i, crypto.Hash{})
		err2 := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(1)}}, uint64(0), i, crypto.Hash{})
		if err := errors.Compose(err1, err2); err != nil {
			t.Fatal(err)
		}
	}
	if f.UploadProgress() != 100 {
		t.Fatal("expected uploadProgress to report 100% but was", f.UploadProgress())
	}
}

// TestFileRedundancy tests that redundancy is correctly calculated for files
// with varying number of filecontracts and erasure code settings.
func TestFileRedundancy(t *testing.T) {
	nDatas := []int{1, 2, 10}
	neverOffline := make(map[string]bool)
	goodForRenew := make(map[string]bool)
	for i := 0; i < 6; i++ {
		neverOffline[string([]byte{byte(i)})] = false
		goodForRenew[string([]byte{byte(i)})] = true
	}

	for _, nData := range nDatas {
		rsc, _ := NewRSCode(nData, 10)
		f := newFile(t.Name(), rsc, 100, 1000, 0777, "")
		// Test that an empty file has 0 redundancy.
		if r := f.Redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that a file with 1 host that has a piece for every chunk but
		// one chunk still has a redundancy of 0.
		for i := uint64(0); i < f.NumChunks()-1; i++ {
			err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(0)}}, i, 0, crypto.Hash{})
			if err != nil {
				t.Fatal(err)
			}
		}
		if r := f.Redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that adding another host with a piece for every chunk but one
		// chunk still results in a file with redundancy 0.
		for i := uint64(0); i < f.NumChunks()-1; i++ {
			err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(1)}}, i, 1, crypto.Hash{})
			if err != nil {
				t.Fatal(err)
			}
		}
		if r := f.Redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that adding a file contract with a piece for the missing chunk
		// results in a file with redundancy > 0 && <= 1.
		err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(2)}}, f.NumChunks()-1, 0, crypto.Hash{})
		if err != nil {
			t.Fatal(err)
		}
		// 1.0 / MinPieces because the chunk with the least number of pieces has 1 piece.
		expectedR := 1.0 / float64(f.ErasureCode(0).MinPieces())
		if r := f.Redundancy(neverOffline, goodForRenew); r != expectedR {
			t.Errorf("expected %f redundancy, got %f", expectedR, r)
		}
		// Test that adding a file contract that has erasureCode.MinPieces() pieces
		// per chunk for all chunks results in a file with redundancy > 1.
		for iChunk := uint64(0); iChunk < f.NumChunks(); iChunk++ {
			for iPiece := uint64(1); iPiece < uint64(f.ErasureCode(0).MinPieces()); iPiece++ {
				err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(3)}}, iChunk, iPiece, crypto.Hash{})
				if err != nil {
					t.Fatal(err)
				}
			}
			err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(4)}}, iChunk, uint64(f.ErasureCode(0).MinPieces()), crypto.Hash{})
			if err != nil {
				t.Fatal(err)
			}
		}
		// 1+MinPieces / MinPieces because the chunk with the least number of pieces has 1+MinPieces pieces.
		expectedR = float64(1+f.ErasureCode(0).MinPieces()) / float64(f.ErasureCode(0).MinPieces())
		if r := f.Redundancy(neverOffline, goodForRenew); r != expectedR {
			t.Errorf("expected %f redundancy, got %f", expectedR, r)
		}

		// verify offline file contracts are not counted in the redundancy
		for iChunk := uint64(0); iChunk < f.NumChunks(); iChunk++ {
			for iPiece := uint64(0); iPiece < uint64(f.ErasureCode(0).MinPieces()); iPiece++ {
				err := f.AddPiece(types.SiaPublicKey{Key: []byte{byte(5)}}, iChunk, iPiece, crypto.Hash{})
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		specificOffline := make(map[string]bool)
		for pk := range goodForRenew {
			specificOffline[pk] = false
		}
		specificOffline[string(byte(5))] = true
		if r := f.Redundancy(specificOffline, goodForRenew); r != expectedR {
			t.Errorf("expected redundancy to ignore offline file contracts, wanted %f got %f", expectedR, r)
		}
	}
}

// TestFileExpiration probes the expiration method of the file type.
func TestFileExpiration(t *testing.T) {
	rsc, _ := NewRSCode(1, 2)
	f := newFile(t.Name(), rsc, pieceSize, 1000, 0777, "")
	contracts := make(map[string]modules.RenterContract)
	if f.Expiration(contracts) != 0 {
		t.Error("file with no pieces should report as having no time remaining")
	}
	// Create 3 public keys
	pk1 := types.SiaPublicKey{Key: []byte{0}}
	pk2 := types.SiaPublicKey{Key: []byte{1}}
	pk3 := types.SiaPublicKey{Key: []byte{2}}

	// Add a piece for each key to the file.
	err1 := f.AddPiece(pk1, 0, 0, crypto.Hash{})
	err2 := f.AddPiece(pk2, 0, 1, crypto.Hash{})
	err3 := f.AddPiece(pk3, 0, 2, crypto.Hash{})
	if err := errors.Compose(err1, err2, err3); err != nil {
		t.Fatal(err)
	}

	// Add a contract.
	fc := modules.RenterContract{}
	fc.EndHeight = 100
	contracts[string(pk1.Key)] = fc
	if f.Expiration(contracts) != 100 {
		t.Error("file did not report lowest WindowStart")
	}

	// Add a contract with a lower WindowStart.
	fc.EndHeight = 50
	contracts[string(pk2.Key)] = fc
	if f.Expiration(contracts) != 50 {
		t.Error("file did not report lowest WindowStart")
	}

	// Add a contract with a higher WindowStart.
	fc.EndHeight = 75
	contracts[string(pk3.Key)] = fc
	if f.Expiration(contracts) != 50 {
		t.Error("file did not report lowest WindowStart")
	}
}

// TestRenterFileListLocalPath verifies that FileList() returns the correct
// local path information for an uploaded file.
func TestRenterFileListLocalPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	id := rt.renter.mu.Lock()
	f := newTestingFile()
	f.SetLocalPath("TestPath")
	rt.renter.files[f.SiaPath()] = f
	rt.renter.persist.Tracking[f.SiaPath()] = trackedFile{
		RepairPath: f.LocalPath(),
	}
	rt.renter.mu.Unlock(id)
	files := rt.renter.FileList()
	if len(files) != 1 {
		t.Fatal("wrong number of files, got", len(files), "wanted one")
	}
	if files[0].LocalPath != "TestPath" {
		t.Fatal("file had wrong LocalPath: got", files[0].LocalPath, "wanted TestPath")
	}
}

// TestRenterDeleteFile probes the DeleteFile method of the renter type.
func TestRenterDeleteFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Delete a file from an empty renter.
	err = rt.renter.DeleteFile("dne")
	if err != ErrUnknownPath {
		t.Error("Expected ErrUnknownPath:", err)
	}

	// Put a file in the renter.
	file1 := newTestingFile()
	rt.renter.files[file1.SiaPath()] = file1
	// Delete a different file.
	err = rt.renter.DeleteFile("one")
	if err != ErrUnknownPath {
		t.Error("Expected ErrUnknownPath, got", err)
	}
	// Delete the file.
	err = rt.renter.DeleteFile(file1.SiaPath())
	if err != nil {
		t.Error(err)
	}
	if len(rt.renter.FileList()) != 0 {
		t.Error("file was deleted, but is still reported in FileList")
	}

	// Put a file in the renter, then rename it.
	f := newTestingFile()
	f.Rename("1") // set name to "1"
	rt.renter.files[f.SiaPath()] = f
	rt.renter.RenameFile(f.SiaPath(), "one")
	// Call delete on the previous name.
	err = rt.renter.DeleteFile("1")
	if err != ErrUnknownPath {
		t.Error("Expected ErrUnknownPath, got", err)
	}
	// Call delete on the new name.
	err = rt.renter.DeleteFile("one")
	if err != nil {
		t.Error(err)
	}

	// Check that all .sia files have been deleted.
	var walkStr string
	filepath.Walk(rt.renter.persistDir, func(path string, _ os.FileInfo, _ error) error {
		// capture only .sia files
		if filepath.Ext(path) == ".sia" {
			rel, _ := filepath.Rel(rt.renter.persistDir, path) // strip testdir prefix
			walkStr += rel
		}
		return nil
	})
	expWalkStr := ""
	if walkStr != expWalkStr {
		t.Fatalf("Bad walk string: expected %q, got %q", expWalkStr, walkStr)
	}
}

// TestRenterFileList probes the FileList method of the renter type.
func TestRenterFileList(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Get the file list of an empty renter.
	if len(rt.renter.FileList()) != 0 {
		t.Error("FileList has non-zero length for empty renter?")
	}

	// Put a file in the renter.
	file1 := newTestingFile()
	rt.renter.files[file1.SiaPath()] = file1
	if len(rt.renter.FileList()) != 1 {
		t.Error("FileList is not returning the only file in the renter")
	}
	if rt.renter.FileList()[0].SiaPath != file1.SiaPath() {
		t.Error("FileList is not returning the correct filename for the only file")
	}

	// Put multiple files in the renter.
	file2 := newTestingFile()
	rt.renter.files["2"] = file2
	if len(rt.renter.FileList()) != 2 {
		t.Error("FileList is not returning both files in the renter")
	}
	files := rt.renter.FileList()
	if !((files[0].SiaPath == file1.SiaPath() || files[0].SiaPath == file2.SiaPath()) &&
		(files[1].SiaPath == file1.SiaPath() || files[1].SiaPath == file2.SiaPath()) &&
		(files[0].SiaPath != files[1].SiaPath)) {
		t.Error("FileList is returning wrong names for the files:", files[0].SiaPath, files[1].SiaPath)
	}
}

// TestRenterRenameFile probes the rename method of the renter.
func TestRenterRenameFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Rename a file that doesn't exist.
	err = rt.renter.RenameFile("1", "1a")
	if err != ErrUnknownPath {
		t.Error("Expecting ErrUnknownPath:", err)
	}

	// Rename a file that does exist.
	f := newTestingFile()
	f.Rename("1")
	rt.renter.files["1"] = f
	err = rt.renter.RenameFile("1", "1a")
	if err != nil {
		t.Fatal(err)
	}
	files := rt.renter.FileList()
	if len(files) != 1 {
		t.Fatal("FileList has unexpected number of files:", len(files))
	}
	if files[0].SiaPath != "1a" {
		t.Errorf("RenameFile failed: expected 1a, got %v", files[0].SiaPath)
	}

	// Rename a file to an existing name.
	f2 := newTestingFile()
	f2.Rename("1")
	rt.renter.files["1"] = f2
	err = rt.renter.RenameFile("1", "1a")
	if err != ErrPathOverload {
		t.Error("Expecting ErrPathOverload, got", err)
	}

	// Rename a file to the same name.
	err = rt.renter.RenameFile("1", "1")
	if err != ErrPathOverload {
		t.Error("Expecting ErrPathOverload, got", err)
	}

	// Renaming should also update the tracking set
	rt.renter.persist.Tracking["1"] = trackedFile{
		RepairPath: f2.LocalPath(),
	}
	err = rt.renter.RenameFile("1", "1b")
	if err != nil {
		t.Fatal(err)
	}
	_, oldexists := rt.renter.persist.Tracking["1"]
	_, newexists := rt.renter.persist.Tracking["1b"]
	if oldexists || !newexists {
		t.Error("renaming should have updated the entry in the tracking set")
	}
}
