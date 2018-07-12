package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	// v112StorageManagerOne names the first legacy storage manager that can be
	// used to test upgrades.
	v112Host = "v112Host.tar.gz"
)

// loadExistingHostWithNewDeps will create all of the dependencies for a host, then load
// the host on top of the given directory.
func loadExistingHostWithNewDeps(modulesDir, hostDir string) (modules.Host, error) {
	testdir := build.TempDir(modules.HostDir, modulesDir)

	// Create the host dependencies.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, err := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}

	// Create the host.
	h, err := newHost(modules.ProdDependencies, cs, g, tp, w, "localhost:0", hostDir)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// TestV112StorageManagerUpgrade creates a host with a legacy storage manager,
// and then attempts to upgrade the storage manager.
func TestV112StorageManagerUpgrade(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Copy the testdir legacy storage manager to the temp directory.
	source := filepath.Join("testdata", v112Host)
	legacyHost := build.TempDir(modules.HostDir, t.Name(), modules.HostDir)
	err := build.ExtractTarGz(source, legacyHost)
	if err != nil {
		t.Fatal(err)
	}

	// Patch the storagemanager.json to point to the new storage folder
	// location.
	smPersist := new(v112StorageManagerPersist)
	err = persist.LoadJSON(v112StorageManagerMetadata, smPersist, filepath.Join(legacyHost, v112StorageManagerDir, v112StorageManagerPersistFilename))
	if err != nil {
		t.Fatal(err)
	}
	smPersist.StorageFolders[0].Path = filepath.Join(legacyHost, "storageFolderOne")
	smPersist.StorageFolders[1].Path = filepath.Join(legacyHost, "storageFolderTwo")
	err = persist.SaveJSON(v112StorageManagerMetadata, smPersist, filepath.Join(legacyHost, v112StorageManagerDir, v112StorageManagerPersistFilename))
	if err != nil {
		t.Fatal(err)
	}
	oldCapacity := smPersist.StorageFolders[0].Size + smPersist.StorageFolders[1].Size
	oldCapacityRemaining := smPersist.StorageFolders[0].SizeRemaining + smPersist.StorageFolders[1].SizeRemaining
	oldUsed := oldCapacity - oldCapacityRemaining

	// Create the symlink to point to the storage folder.
	err = os.Symlink(filepath.Join(legacyHost, "storageFolderOne"), filepath.Join(legacyHost, v112StorageManagerDir, "66"))
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(filepath.Join(legacyHost, "storageFolderTwo"), filepath.Join(legacyHost, v112StorageManagerDir, "04"))
	if err != nil {
		t.Fatal(err)
	}

	// Patching complete. Proceed to create the host and verify that the
	// upgrade went smoothly.
	host, err := loadExistingHostWithNewDeps(filepath.Join(t.Name(), "newDeps"), legacyHost)
	if err != nil {
		t.Fatal(err)
	}

	storageFolders := host.StorageFolders()
	if len(storageFolders) != 2 {
		t.Fatal("Storage manager upgrade was unsuccessful.")
	}

	// The amount of data reported should match the previous amount of data
	// that was stored.
	capacity := storageFolders[0].Capacity + storageFolders[1].Capacity
	capacityRemaining := storageFolders[0].CapacityRemaining + storageFolders[1].CapacityRemaining
	capacityUsed := capacity - capacityRemaining
	if capacity != oldCapacity {
		t.Error("new storage folders don't have the same size as the old storage folders")
	}
	if capacityRemaining != oldCapacityRemaining {
		t.Error("capacity remaining statistics do not match up", capacityRemaining/modules.SectorSize, oldCapacityRemaining/modules.SectorSize)
	}
	if oldUsed != capacityUsed {
		t.Error("storage folders have different usage values", capacityUsed/modules.SectorSize, oldUsed/modules.SectorSize)
	}
}
