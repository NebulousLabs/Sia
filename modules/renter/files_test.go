package renter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
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
		f := &file{size: test.size, erasureCode: rsc, pieceSize: test.pieceSize}
		if f.numChunks() != test.expNumChunks {
			t.Errorf("Test %v: expected %v, got %v", test, test.expNumChunks, f.numChunks())
		}
	}
}

// TestFileAvailable probes the available method of the file type.
func TestFileAvailable(t *testing.T) {
	rsc, _ := NewRSCode(1, 10)
	f := &file{
		size:        1000,
		erasureCode: rsc,
		pieceSize:   100,
	}
	neverOffline := make(map[types.FileContractID]bool)

	if f.available(neverOffline) {
		t.Error("file should not be available")
	}

	var fc fileContract
	for i := uint64(0); i < f.numChunks(); i++ {
		fc.Pieces = append(fc.Pieces, pieceData{Chunk: i, Piece: 0})
	}
	f.contracts = map[types.FileContractID]fileContract{{}: fc}

	if !f.available(neverOffline) {
		t.Error("file should be available")
	}

	specificOffline := make(map[types.FileContractID]bool)
	specificOffline[fc.ID] = true
	if f.available(specificOffline) {
		t.Error("file should not be available")
	}
}

// TestFileUploadedBytes tests that uploadedBytes() returns a value equal to
// the number of sectors stored via contract times the size of each sector.
func TestFileUploadedBytes(t *testing.T) {
	f := &file{}
	// ensure that a piece fits within a sector
	f.pieceSize = modules.SectorSize / 2
	f.contracts = make(map[types.FileContractID]fileContract)
	f.contracts[types.FileContractID{}] = fileContract{
		ID:     types.FileContractID{},
		IP:     modules.NetAddress(""),
		Pieces: make([]pieceData, 4),
	}
	if f.uploadedBytes() != 4*modules.SectorSize {
		t.Errorf("expected uploadedBytes to be 8, got %v", f.uploadedBytes())
	}
}

// TestFileUploadProgressPinning verifies that uploadProgress() returns at most
// 100%, even if more pieces have been uploaded,
func TestFileUploadProgressPinning(t *testing.T) {
	f := &file{}
	f.pieceSize = 2
	f.contracts = make(map[types.FileContractID]fileContract)
	f.contracts[types.FileContractID{}] = fileContract{
		ID:     types.FileContractID{},
		IP:     modules.NetAddress(""),
		Pieces: make([]pieceData, 4),
	}
	rsc, _ := NewRSCode(1, 1)
	f.erasureCode = rsc
	if f.uploadProgress() != 100 {
		t.Fatal("expected uploadProgress to report 100%")
	}
}

// TestFileRedundancy tests that redundancy is correctly calculated for files
// with varying number of filecontracts and erasure code settings.
func TestFileRedundancy(t *testing.T) {
	nDatas := []int{1, 2, 10}
	neverOffline := make(map[types.FileContractID]bool)
	goodForRenew := make(map[types.FileContractID]bool)
	for i := 0; i < 5; i++ {
		neverOffline[types.FileContractID{byte(i)}] = false
		goodForRenew[types.FileContractID{byte(i)}] = true
	}

	for _, nData := range nDatas {
		rsc, _ := NewRSCode(nData, 10)
		f := &file{
			size:        1000,
			pieceSize:   100,
			contracts:   make(map[types.FileContractID]fileContract),
			erasureCode: rsc,
		}
		// Test that an empty file has 0 redundancy.
		if r := f.redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that a file with 1 filecontract that has a piece for every chunk but
		// one chunk still has a redundancy of 0.
		fc := fileContract{
			ID: types.FileContractID{0},
		}
		for i := uint64(0); i < f.numChunks()-1; i++ {
			pd := pieceData{
				Chunk: i,
				Piece: 0,
			}
			fc.Pieces = append(fc.Pieces, pd)
		}
		f.contracts[fc.ID] = fc
		if r := f.redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that adding another filecontract with a piece for every chunk but one
		// chunk still results in a file with redundancy 0.
		fc = fileContract{
			ID: types.FileContractID{1},
		}
		for i := uint64(0); i < f.numChunks()-1; i++ {
			pd := pieceData{
				Chunk: i,
				Piece: 1,
			}
			fc.Pieces = append(fc.Pieces, pd)
		}
		f.contracts[fc.ID] = fc
		if r := f.redundancy(neverOffline, goodForRenew); r != 0 {
			t.Error("expected 0 redundancy, got", r)
		}
		// Test that adding a file contract with a piece for the missing chunk
		// results in a file with redundancy > 0 && <= 1.
		fc = fileContract{
			ID: types.FileContractID{2},
		}
		pd := pieceData{
			Chunk: f.numChunks() - 1,
			Piece: 0,
		}
		fc.Pieces = append(fc.Pieces, pd)
		f.contracts[fc.ID] = fc
		// 1.0 / MinPieces because the chunk with the least number of pieces has 1 piece.
		expectedR := 1.0 / float64(f.erasureCode.MinPieces())
		if r := f.redundancy(neverOffline, goodForRenew); r != expectedR {
			t.Errorf("expected %f redundancy, got %f", expectedR, r)
		}
		// Test that adding a file contract that has erasureCode.MinPieces() pieces
		// per chunk for all chunks results in a file with redundancy > 1.
		fc = fileContract{
			ID: types.FileContractID{3},
		}
		for iChunk := uint64(0); iChunk < f.numChunks(); iChunk++ {
			for iPiece := uint64(0); iPiece < uint64(f.erasureCode.MinPieces()); iPiece++ {
				fc.Pieces = append(fc.Pieces, pieceData{
					Chunk: iChunk,
					// add 1 since the same piece can't count towards redundancy twice.
					Piece: iPiece + 1,
				})
			}
		}
		f.contracts[fc.ID] = fc
		// 1+MinPieces / MinPieces because the chunk with the least number of pieces has 1+MinPieces pieces.
		expectedR = float64(1+f.erasureCode.MinPieces()) / float64(f.erasureCode.MinPieces())
		if r := f.redundancy(neverOffline, goodForRenew); r != expectedR {
			t.Errorf("expected %f redundancy, got %f", expectedR, r)
		}

		// verify offline file contracts are not counted in the redundancy
		fc = fileContract{
			ID: types.FileContractID{4},
		}
		for iChunk := uint64(0); iChunk < f.numChunks(); iChunk++ {
			for iPiece := uint64(0); iPiece < uint64(f.erasureCode.MinPieces()); iPiece++ {
				fc.Pieces = append(fc.Pieces, pieceData{
					Chunk: iChunk,
					Piece: iPiece,
				})
			}
		}
		f.contracts[fc.ID] = fc
		specificOffline := make(map[types.FileContractID]bool)
		for fcid := range goodForRenew {
			specificOffline[fcid] = false
		}
		specificOffline[fc.ID] = true
		if r := f.redundancy(specificOffline, goodForRenew); r != expectedR {
			t.Errorf("expected redundancy to ignore offline file contracts, wanted %f got %f", expectedR, r)
		}
	}
}

// TestFileExpiration probes the expiration method of the file type.
func TestFileExpiration(t *testing.T) {
	f := &file{
		contracts: make(map[types.FileContractID]fileContract),
	}

	if f.expiration() != 0 {
		t.Error("file with no pieces should report as having no time remaining")
	}

	// Add a contract.
	fc := fileContract{}
	fc.WindowStart = 100
	f.contracts[types.FileContractID{0}] = fc
	if f.expiration() != 100 {
		t.Error("file did not report lowest WindowStart")
	}

	// Add a contract with a lower WindowStart.
	fc.WindowStart = 50
	f.contracts[types.FileContractID{1}] = fc
	if f.expiration() != 50 {
		t.Error("file did not report lowest WindowStart")
	}

	// Add a contract with a higher WindowStart.
	fc.WindowStart = 75
	f.contracts[types.FileContractID{2}] = fc
	if f.expiration() != 50 {
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
	f.name = "testname"
	rt.renter.files["test"] = f
	rt.renter.persist.Tracking[f.name] = trackedFile{
		RepairPath: "TestPath",
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
	rt.renter.files["1"] = &file{
		name: "one",
	}
	// Delete a different file.
	err = rt.renter.DeleteFile("one")
	if err != ErrUnknownPath {
		t.Error("Expected ErrUnknownPath, got", err)
	}
	// Delete the file.
	err = rt.renter.DeleteFile("1")
	if err != nil {
		t.Error(err)
	}
	if len(rt.renter.FileList()) != 0 {
		t.Error("file was deleted, but is still reported in FileList")
	}

	// Put a file in the renter, then rename it.
	f := newTestingFile()
	f.name = "1"
	rt.renter.files[f.name] = f
	rt.renter.RenameFile(f.name, "one")
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
	rsc, _ := NewRSCode(1, 1)
	rt.renter.files["1"] = &file{
		name:        "one",
		erasureCode: rsc,
		pieceSize:   1,
	}
	if len(rt.renter.FileList()) != 1 {
		t.Error("FileList is not returning the only file in the renter")
	}
	if rt.renter.FileList()[0].SiaPath != "one" {
		t.Error("FileList is not returning the correct filename for the only file")
	}

	// Put multiple files in the renter.
	rt.renter.files["2"] = &file{
		name:        "two",
		erasureCode: rsc,
		pieceSize:   1,
	}
	if len(rt.renter.FileList()) != 2 {
		t.Error("FileList is not returning both files in the renter")
	}
	files := rt.renter.FileList()
	if !((files[0].SiaPath == "one" || files[0].SiaPath == "two") &&
		(files[1].SiaPath == "one" || files[1].SiaPath == "two") &&
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
	f.name = "1"
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
	f2.name = "1"
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
	rt.renter.persist.Tracking["1"] = trackedFile{"foo"}
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
