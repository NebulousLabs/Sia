package contractmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestGrowStorageFolder checks that a storage folder can be successfully
// increased in size.
func TestGrowStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestGrowStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder.
	storageFolderOne := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderOne, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderOne, modules.SectorSize*storageFolderGranularity)
	if err != nil {
		t.Fatal(err)
	}

	// Get the index of the storage folder.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("there should only be one storage folder")
	}
	sfIndex := sfs[0].Index
	// Verify that the storage folder has the correct capacity.
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	// Verify that the on-disk files are the right size.
	mfn := filepath.Join(storageFolderOne, metadataFile)
	sfn := filepath.Join(storageFolderOne, sectorFile)
	mfi, err := os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err := os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity {
		t.Error("sector file is the wrong size")
	}

	// Increase the size of the storage folder.
	err = cmt.cm.ResizeStorageFolder(sfIndex, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}

	// Restart the contract manager to see that the change is persistent.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the capacity and file sizes are correct.
	sfs = cmt.cm.StorageFolders()
	if sfs[0].Capacity != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("new storage folder is reporting the wrong capacity")
	}
	mfi, err = os.Stat(mfn)
	if err != nil {
		t.Fatal(err)
	}
	sfi, err = os.Stat(sfn)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(mfi.Size()) != sectorMetadataDiskSize*storageFolderGranularity*2 {
		t.Error("metadata file is the wrong size")
	}
	if uint64(sfi.Size()) != modules.SectorSize*storageFolderGranularity*2 {
		t.Error("sector file is the wrong size")
	}
}

// TODO: Use an interrupt that will prevent the storage folder from completing
// the writes, simulating unclean shutdown.

// TODO: Use an interrupt that will preven the storage folder from saving all
// the way, but get it far enough that the completed post is in the WAL, such
// that recoverWAL will restore the resize as it as completed.
