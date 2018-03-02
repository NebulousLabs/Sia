package contractmanager

// TODO: Verify that the code gracefully handles multiple storage folders
// failiing, as well as all of them failing.

// Verify that the actual data stored on disk matches the sector roots that it
// is suppsed to match. Especially for multi-storage folder,
// post-resize/remove, after renews and exirations (involving viritual
// contracts), with large enough amounts of data that storage folders sometimes
// filled up entirely (multiple pages of storageFolderGranularity size).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/fastrand"
)

// randSector creates a random sector that can be added to the contract
// manager.
func randSector() (root crypto.Hash, data []byte) {
	data = fastrand.Bytes(int(modules.SectorSize))
	root = crypto.MerkleRoot(data)
	return
}

// TestAddSector tries to add a sector to the contract manager, blocking until
// the add has completed.
func TestAddSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddSector")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
	sectorData, err := cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sectorData, data) {
		t.Fatal("wrong sector provided")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
	sectorData, err = cmt.cm.ReadSector(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sectorData, data) {
		t.Fatal("wrong sector provided")
	}
}

// TestAddSectorFillFolder adds sectors to a 64 sector storage folder until it
// is full.
func TestAddSectorFillFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddSectorFillFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*storageFolderGranularity)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate 65 sectors for the storage folder, which can only hold 64.
	roots := make([]crypto.Hash, storageFolderGranularity+1)
	datas := make([][]byte, storageFolderGranularity+1)
	var wg sync.WaitGroup
	for i := 0; i < storageFolderGranularity+1; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			root, data := randSector()
			roots[i] = root
			datas[i] = data
		}(i)
	}
	wg.Wait()

	// Add 64 sectors which should fit cleanly. The sectors are added in
	// parallel to make use of the batching in the contract manager.
	for i := 0; i < storageFolderGranularity; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	wg.Wait()

	// Check that the sectors were successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].CapacityRemaining != 0 {
		t.Error("Storage folder is supposed to be full", sfs[0].CapacityRemaining)
	}

	// Try adding a 65th sector, it should not succeed.
	err = cmt.cm.AddSector(roots[storageFolderGranularity], datas[storageFolderGranularity])
	if err == nil {
		t.Error("expecting the storage folder to be full.")
	}

	// Try reading each sector.
	for i := range roots[:storageFolderGranularity] {
		data, err := cmt.cm.ReadSector(roots[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("Contract manager returned the wrong data on a sector request")
		}
	}
}

// TestAddSectorFillLargerFolder adds sectors to a 128 sector storage folder
// until it is full.
func TestAddSectorFillLargerFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddSectorFillLargerFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*storageFolderGranularity*2)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate 65 sectors for the storage folder, which can only hold 64.
	roots := make([]crypto.Hash, storageFolderGranularity*2+1)
	datas := make([][]byte, storageFolderGranularity*2+1)
	var wg sync.WaitGroup
	for i := 0; i < storageFolderGranularity*2+1; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			root, data := randSector()
			roots[i] = root
			datas[i] = data
		}(i)
	}
	wg.Wait()

	// Add 64 sectors which should fit cleanly. The sectors are added in
	// parallel to make use of the batching in the contract manager.
	for i := 0; i < storageFolderGranularity*2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := cmt.cm.AddSector(roots[i], datas[i])
			if err != nil {
				t.Fatal(err)
			}
		}(i)
	}
	wg.Wait()

	// Check that the sectors were successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].CapacityRemaining != 0 {
		t.Error("Storage folder is supposed to be full", sfs[0].CapacityRemaining)
	}

	// Try adding a 65th sector, it should not succeed.
	err = cmt.cm.AddSector(roots[storageFolderGranularity*2], datas[storageFolderGranularity*2])
	if err == nil {
		t.Error("expecting the storage folder to be full.")
	}

	// Try reading each sector.
	for i := range roots[:storageFolderGranularity*2] {
		data, err := cmt.cm.ReadSector(roots[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, datas[i]) {
			t.Error("Contract manager returned the wrong data on a sector request")
		}
	}
}

// dependencyNoSettingsSave is a mocked dependency that will prevent the
// settings file from saving.
type dependencyNoSettingsSave struct {
	modules.ProductionDependencies
	triggered bool
	mu        sync.Mutex
}

// disrupt will disrupt the threadedSyncLoop, causing the loop to terminate as
// soon as it is created.
func (d *dependencyNoSettingsSave) Disrupt(s string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if s == "settingsSyncRename" && d.triggered {
		// Prevent the settings file from being saved.
		return true
	}
	if s == "walRename" && d.triggered {
		// Prevent the WAL from being renamed, which will prevent the existing
		// WAL from being overwritten.
		return true
	}
	if s == "cleanWALFile" {
		// Prevent the WAL file from being removed.
		return true
	}
	return false
}

// TestAddSectorRecovery checks that the WAL recovery of an added sector is
// graceful / correct in the event of unclean shutdown.
func TestAddSectorRecovery(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyNoSettingsSave)
	cmt, err := newMockedContractManagerTester(d, "TestAddSectorRecovery")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data := randSector()
	// Disrupt the sync loop before adding the sector, such that the add sector
	// call makes it into the WAL but not into the saved settings.
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	d.triggered = true
	d.mu.Unlock()

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
		if sf.sectors != 1 {
			t.Error("the number of sectors is being counted incorrectly")
		}
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", (sfs[0].Capacity-sfs[0].CapacityRemaining)/modules.SectorSize)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
		if sf.sectors != 1 {
			t.Error("the number of sectors is being counted incorrectly")
		}
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestAddVirtualSectorSerial adds a sector and a virual sector in serial to
// the contract manager.
func TestAddVirtualSectorSerial(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddVirtualSectorSerial")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data := randSector()
	// Add the sector twice in serial to verify that virtual sector adding is
	// working correctly.
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 2 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 2 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestAddVirtualSectorParallel adds a sector and a virual sector in parallel
// to the contract manager.
func TestAddVirtualSectorParallel(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddVirtualSectorParallel")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data := randSector()
	// Add the sector twice in serial to verify that virtual sector adding is
	// working correctly.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := cmt.cm.AddSector(root, data)
		if err != nil {
			t.Fatal(err)
		}
	}()
	go func() {
		defer wg.Done()
		err := cmt.cm.AddSector(root, data)
		if err != nil {
			t.Fatal(err)
		}
	}()
	wg.Wait()

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 2 {
			t.Error("Sector location should be reporting a count of 2 for this sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 2 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestAddVirtualSectorMassiveParallel adds the same sector many times in
// parallel to the contract manager.
func TestAddVirtualSectorMassiveParallel(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestAddVirtualSectorMassiveParallel")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Fabricate a sector and add it to the contract manager.
	root, data := randSector()
	// Add the sector many times in parallel to make sure it is handled
	// gracefully.
	var wg sync.WaitGroup
	parallelAdds := uint16(20)
	for i := uint16(0); i < parallelAdds; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cmt.cm.AddSector(root, data)
			if err != nil {
				t.Fatal(err)
			}
		}()
	}
	wg.Wait()

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != parallelAdds {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != parallelAdds {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestRemoveSector tries to remove a sector from the contract manager.
func TestRemoveSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestRemoveSector")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Add two sectors, and then remove one of them.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	root2, data2 := randSector()
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.RemoveSector(root2)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", (sfs[0].Capacity-sfs[0].CapacityRemaining)/modules.SectorSize)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map:", len(cmt.cm.sectorLocations))
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestRemoveSectorVirtual tries to remove a virtual sector from the contract
// manager.
func TestRemoveSectorVirtual(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestRemoveSectorVirtual")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Add a physical sector, then a virtual sector, and then remove the
	// virtual one.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.RemoveSector(root)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestDeleteSector tries to delete a sector from the contract manager.
func TestDeleteSector(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestDeleteSector")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Add two sectors, and then delete one of them.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	root2, data2 := randSector()
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.DeleteSector(root2)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity/modules.SectorSize, sfs[0].CapacityRemaining/modules.SectorSize)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestDeleteSectorVirtual tries to delete a sector with virtual pieces from
// the contract manager.
func TestDeleteSectorVirtual(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestDeleteSectorVirtual")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	// Create the storage folder dir.
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}

	// Add two sectors, and then delete one of them.
	root, data := randSector()
	err = cmt.cm.AddSector(root, data)
	if err != nil {
		t.Fatal(err)
	}
	root2, data2 := randSector()
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddSector(root2, data2)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.DeleteSector(root2)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder")
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found := false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the sector was successfully added.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 1 {
		t.Fatal("There should be one storage folder in the contract manager", len(sfs))
	}
	if sfs[0].Capacity != sfs[0].CapacityRemaining+modules.SectorSize {
		t.Error("One sector's worth of capacity should be consumed:", sfs[0].Capacity, sfs[0].CapacityRemaining)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 1 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 1 {
		t.Fatal("storage folder not being reported correctly")
	}
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector:", sl.count)
		}
		if sl.storageFolder != index {
			t.Error("Sector location is being reported incorrectly - wrong storage folder", sl.storageFolder, index)
		}
		if sl.index > 64 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
	// Check the usage.
	found = false
	for _, u := range cmt.cm.storageFolders[index].usage {
		if u != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("usage field does not seem to have been updated")
	}
}

// TestSectorBalancing checks that the contract manager evenly balances sectors
// between storage folders.
func TestSectorBalancing(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cmt, err := newContractManagerTester("TestSectorBalancing")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}
	// Add a second storage folder.
	storageFolderDir2 := filepath.Join(cmt.persistDir, "storageFolderTwo")
	err = os.MkdirAll(storageFolderDir2, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir2, modules.SectorSize*64)
	if err != nil {
		t.Fatal(err)
	}
	// Add a third storage folder, twice as large.
	storageFolderDir3 := filepath.Join(cmt.persistDir, "storageFolderThree")
	err = os.MkdirAll(storageFolderDir3, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir3, modules.SectorSize*64*2)
	if err != nil {
		t.Fatal(err)
	}

	// Add 20 sectors.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cmt.cm.AddSector(randSector())
			if err != nil {
				t.Fatal(err)
			}
		}()
	}
	wg.Wait()

	// Verify that that all 20 sectors were accepted.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 3 {
		t.Fatal("There should be two storage folders in the contract manager", len(sfs))
	}
	if sfs[0].Capacity+sfs[1].Capacity+sfs[2].Capacity != sfs[0].CapacityRemaining+sfs[1].CapacityRemaining+sfs[2].CapacityRemaining+modules.SectorSize*20 {
		t.Error("sectors do not appear to have been added correctly")
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 20 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 3 {
		t.Fatal("storage folder not being reported correctly")
	}
	// Check a storage folder at random, verify that the sectors are sane.
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.storageFolder != index {
			continue
		}
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.index > 64*2 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that that all 20 sectors were accepted, and that they have been
	// distributed evenly between storage folders.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 3 {
		t.Fatal("There should be two storage folders in the contract manager", len(sfs))
	}
	if sfs[0].Capacity+sfs[1].Capacity+sfs[2].Capacity != sfs[0].CapacityRemaining+sfs[1].CapacityRemaining+sfs[2].CapacityRemaining+modules.SectorSize*20 {
		t.Error("sectors do not appear to have been added correctly")
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 20 {
		t.Fatal("there should be twenty sectors reported in the sectorLocations map:", len(cmt.cm.sectorLocations))
	}
	if len(cmt.cm.storageFolders) != 3 {
		t.Fatal("storage folder not being reported correctly")
	}
	// Check a storage folder at random, verify that the sectors are sane.
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.storageFolder != index {
			continue
		}
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.index > 64*2 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
}

// dependencyFailingWrites is a mocked dependency that will prevent file writes
// for some files.
type dependencyFailingWrites struct {
	modules.ProductionDependencies
	triggered *bool
	mu        *sync.Mutex
}

// failureProneFile will begin returning failures on Write for files with
// names/paths containing the string "storageFolderOne" after d.triggered has
// been set to "true".
type failureProneFile struct {
	triggered *bool
	mu        *sync.Mutex
	*os.File
	sync.Mutex
}

// createFile will return a file which will cause errors on Write calls if
// "storageFolderOne" is in the filepath.
func (d *dependencyFailingWrites) CreateFile(s string) (modules.File, error) {
	osfile, err := os.Create(s)
	if err != nil {
		return nil, err
	}

	fpf := &failureProneFile{
		triggered: d.triggered,
		mu:        d.mu,
		File:      osfile,
	}
	return fpf, nil
}

// Write returns an error if the errors in the dependency have been triggered,
// and if this file belongs to "storageFolderOne".
func (fpf *failureProneFile) WriteAt(b []byte, offset int64) (int, error) {
	fpf.mu.Lock()
	triggered := *fpf.triggered
	fpf.mu.Unlock()

	name := fpf.Name()
	if triggered && strings.Contains(name, "storageFolderOne") {
		return 0, errors.New("storage folder is failing")
	}
	return fpf.File.WriteAt(b, offset)
}

// TestFailingStorageFolder checks that the contract manager can continue when
// a storage folder is failing.
func TestFailingStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	d := new(dependencyFailingWrites)
	d.mu = new(sync.Mutex)
	d.triggered = new(bool)
	cmt, err := newMockedContractManagerTester(d, "TestFailingStorageFolder")
	if err != nil {
		t.Fatal(err)
	}
	defer cmt.panicClose()

	// Add a storage folder to the contract manager tester.
	storageFolderDir := filepath.Join(cmt.persistDir, "storageFolderOne")
	err = os.MkdirAll(storageFolderDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir, modules.SectorSize*64*2)
	if err != nil {
		t.Fatal(err)
	}
	// Add a second storage folder.
	storageFolderDir2 := filepath.Join(cmt.persistDir, "storageFolderTwo")
	err = os.MkdirAll(storageFolderDir2, 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = cmt.cm.AddStorageFolder(storageFolderDir2, modules.SectorSize*64*2)
	if err != nil {
		t.Fatal(err)
	}

	// Add 50 sectors.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cmt.cm.AddSector(randSector())
			if err != nil {
				t.Fatal(err)
			}
		}()
	}
	wg.Wait()

	// Verify that that all 20 sectors were accepted, and that they have been
	// distributed evenly between storage folders.
	sfs := cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be two storage folders in the contract manager", len(sfs))
	}
	if sfs[0].Capacity+sfs[1].Capacity != sfs[0].CapacityRemaining+sfs[1].CapacityRemaining+modules.SectorSize*50 {
		t.Error("expecting 20 sectors consumed:", sfs[0].Capacity+sfs[1].Capacity, sfs[0].CapacityRemaining+sfs[1].CapacityRemaining-modules.SectorSize*50)
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 50 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 2 {
		t.Fatal("storage folder not being reported correctly")
	}
	// Check a storage folder at random, verify that the sectors are sane.
	var index uint16
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.storageFolder != index {
			continue
		}
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.index > 128 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}

	// Trigger one of the storage folders to begin failing.
	d.mu.Lock()
	*d.triggered = true
	d.mu.Unlock()

	// Add 50 more sectors.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cmt.cm.AddSector(randSector())
			if err != nil {
				t.Fatal(err)
			}
		}()
	}
	wg.Wait()

	// Verify that that all 20 sectors were accepted, and that they have been
	// added to storageFolderTwo.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be two storage folders in the contract manager", len(sfs))
	}
	if strings.Contains(sfs[0].Path, "storageFolderTwo") {
		// sfs[0] is the working one, should have strictly more than 50
		// sectors.
		if sfs[0].CapacityRemaining+modules.SectorSize*50 >= sfs[0].Capacity {
			t.Error("expecting more than 50 sectors in sfs0")
		}
		if sfs[1].CapacityRemaining+modules.SectorSize*50 <= sfs[1].Capacity {
			t.Error("expecting less than 50 sectors in sfs1")
		}
		if sfs[1].FailedWrites == 0 {
			t.Error("failed write not reported in storage folder stats")
		}
	} else {
		// sfs[1] is the working one, should have strictly more than 50
		// sectors.
		if sfs[1].CapacityRemaining+modules.SectorSize*50 >= sfs[1].Capacity {
			t.Error("expecting more than 50 sectors in sfs1")
		}
		if sfs[0].CapacityRemaining+modules.SectorSize*50 <= sfs[0].Capacity {
			t.Error("expecting less than 50 sectors in sfs0")
		}
		if sfs[0].FailedWrites == 0 {
			t.Error("failed write not reported in storage folder stats")
		}
	}
	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 100 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 2 {
		t.Fatal("storage folder not being reported correctly")
	}
	// Check a storage folder at random, verify that the sectors are sane.
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.storageFolder != index {
			continue
		}
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.index > 128 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}

	// Try reloading the contract manager and see if all of the stateful checks
	// still hold.
	err = cmt.cm.Close()
	if err != nil {
		t.Fatal(err)
	}
	cmt.cm, err = New(filepath.Join(cmt.persistDir, modules.ContractManagerDir))
	if err != nil {
		t.Fatal(err)
	}

	// Verify that that all 20 sectors were accepted, and that they have been
	// added to storageFolderTwo.
	sfs = cmt.cm.StorageFolders()
	if len(sfs) != 2 {
		t.Fatal("There should be two storage folders in the contract manager", len(sfs))
	}
	if strings.Contains(sfs[0].Path, "storageFolderTwo") {
		// sfs[0] is the working one, should have strictly more than 50
		// sectors.
		if sfs[0].CapacityRemaining+modules.SectorSize*50 >= sfs[0].Capacity {
			t.Error("expecting more than 50 sectors in sfs0")
		}
		if sfs[1].CapacityRemaining+modules.SectorSize*50 <= sfs[1].Capacity {
			t.Error("expecting less than 50 sectors in sfs1")
		}
	} else {
		// sfs[1] is the working one, should have strictly more than 50
		// sectors.
		if sfs[1].CapacityRemaining+modules.SectorSize*50 >= sfs[1].Capacity {
			t.Error("expecting more than 50 sectors in sfs1")
		}
		if sfs[0].CapacityRemaining+modules.SectorSize*50 <= sfs[0].Capacity {
			t.Error("expecting less than 50 sectors in sfs0")
		}
	}

	// Break the rules slightly - make the test brittle by looking at the
	// internals directly to determine that the sector got added to the right
	// locations, and that the usage information was updated correctly.
	if len(cmt.cm.sectorLocations) != 100 {
		t.Fatal("there should be one sector reported in the sectorLocations map")
	}
	if len(cmt.cm.storageFolders) != 2 {
		t.Fatal("storage folder not being reported correctly")
	}
	// Check a storage folder at random, verify that the sectors are sane.
	for _, sf := range cmt.cm.storageFolders {
		index = sf.index
	}
	for _, sl := range cmt.cm.sectorLocations {
		if sl.storageFolder != index {
			continue
		}
		if sl.count != 1 {
			t.Error("Sector location should only be reporting one sector")
		}
		if sl.index > 128 {
			t.Error("sector index within storage folder also being reported incorrectly")
		}
	}
}
