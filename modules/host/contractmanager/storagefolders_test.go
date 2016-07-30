package contractmanager

/*
import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// TestStorageFolderUIDString probes the uidString method of the storage
// folder.
func TestStorageFolderUIDString(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a series of uid->string mappings that represent the expected
	// output of calling uidString on a storage folder.
	trials := []struct {
		uid []byte
		str string
	}{
		{
			[]byte{0},
			"00",
		},
		{
			[]byte{255},
			"ff",
		},
		{
			[]byte{50},
			"32",
		},
		{
			[]byte{61},
			"3d",
		},
		{
			[]byte{248},
			"f8",
		},
	}
	for _, trial := range trials {
		sf := &storageFolder{
			UID: trial.uid,
		}
		str := sf.uidString()
		if str != trial.str {
			t.Error("failed UID string trial", str, sf.uidString())
		}
	}
}

// TestStorageFolderUIDStringSanity probes the sanity checks of the uidString
// method of the storage folder.
func TestStorageFolderUIDStringSanity(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a storage folder with an illegal UID size.
	sf := &storageFolder{
		UID: []byte{0, 1},
	}
	// Catch the resulting panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("sanity check was not triggered upon incorrect usage of uidString")
		}
	}()
	_ = sf.uidString()
}

// TestAddStorageFolderUIDCollisions checks that storage folders can be added
// with no risk of producing collisions in the storage folder UIDs. This test
// relies on (explicitly checked) assumptions about the size of the name and
// the number of allowed storage folders.
func TestAddStorageFolderUIDCollisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestAddStorageFolderUIDCollisions")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	// Check that the environment requirements for the test have been met.
	if storageFolderUIDSize != 1 {
		t.Fatal("For this test, the storage manager must be using storage folder UIDs that are 1 byte")
	}
	if maximumStorageFolders < 100 {
		t.Fatal("For this test, the storage manager must be allowed to have at least 100 storage folders")
	}

	// Create 100 storage folders, and check that there are no collisions
	// between any of them. Because the UID is only using 1 byte, once there
	// are more than 64 there will be at least 1/4 chance of a collision for
	// each randomly selected UID. Running into collisions is virtually
	// guaranteed, and running into repeated collisions (where two UIDs
	// consecutively collide with existing UIDs) are highly likely.
	for i := 0; i < maximumStorageFolders; i++ {
		err = smt.addRandFolder(minimumStorageFolderSize)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Check that there are no collisions.
	uidMap := make(map[uint8]struct{})
	for _, sf := range smt.sm.storageFolders {
		_, exists := uidMap[uint8(sf.UID[0])]
		if exists {
			t.Error("Collision")
		}
		uidMap[uint8(sf.UID[0])] = struct{}{}
	}
	// For coverage purposes, try adding a storage folder after the maximum
	// number of storage folders has been reached.
	err = smt.addRandFolder(minimumStorageFolderSize)
	if err != errMaxStorageFolders {
		t.Fatal("expecting errMaxStorageFolders:", err)
	}

	// Try again, this time removing a random storage folder and then adding
	// another one repeatedly - enough times to exceed the 256 possible folder
	// UIDs that can be chosen in the testing environment.
	for i := 0; i < 300; i++ {
		// Replace the very first storage folder.
		err = smt.sm.RemoveStorageFolder(0, false)
		if err != nil {
			t.Fatal(err)
		}
		err = smt.addRandFolder(minimumStorageFolderSize)
		if err != nil {
			t.Fatal(err)
		}

		// Replace a random storage folder.
		n, err := crypto.RandIntn(100)
		if err != nil {
			t.Fatal(err)
		}
		err = smt.sm.RemoveStorageFolder(n, false)
		if err != nil {
			t.Fatal(err)
		}
		err = smt.addRandFolder(minimumStorageFolderSize)
		if err != nil {
			t.Fatal(err)
		}
	}
	uidMap = make(map[uint8]struct{})
	for _, sf := range smt.sm.storageFolders {
		_, exists := uidMap[uint8(sf.UID[0])]
		if exists {
			t.Error("Collision")
		}
		uidMap[uint8(sf.UID[0])] = struct{}{}
	}
}

// TestEmptiestStorageFolder checks that emptiestStorageFolder will correctly
// select the emptiest storage folder out of a provided list of storage
// folders.
func TestEmptiestStorageFolder(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a series of uid->string mappings that represent the expected
	// output of calling uidString on a storage folder.
	trials := []struct {
		folders       []*storageFolder
		emptiestIndex int
	}{
		// Empty input.
		{
			[]*storageFolder{},
			-1,
		},
		// Single valid storage folder.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: minimumStorageFolderSize,
				},
			},
			0,
		},
		// Single full storage folder.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: 0,
				},
			},
			-1,
		},
		// Single nearly full storage folder.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize - 1,
				},
			},
			-1,
		},
		// Two valid storage folders, first is emptier.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize + 1,
				},
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize,
				},
			},
			0,
		},
		// Two valid storage folders, second is emptier.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize,
				},
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize + 1,
				},
			},
			1,
		},
		// Two valid storage folders, first is emptier by percentage but can't
		// hold a new sector.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize - 1,
				},
				{
					Size:          minimumStorageFolderSize * 5,
					SizeRemaining: modules.SectorSize,
				},
			},
			1,
		},
		// Two valid storage folders, first is emptier by volume but not
		// percentage.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: modules.SectorSize * 2,
				},
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize,
				},
			},
			1,
		},
		// Two valid storage folders, second is emptier by volume but not
		// percentage.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize,
				},
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: modules.SectorSize * 2,
				},
			},
			0,
		},
		// Three valid storage folders, second is emptier by percentage but not
		// volume.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: modules.SectorSize * 2,
				},
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: modules.SectorSize,
				},
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: modules.SectorSize * 2,
				},
			},
			1,
		},
		// Three storage folders, none have room for a sector.
		{
			[]*storageFolder{
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: modules.SectorSize - 1,
				},
				{
					Size:          minimumStorageFolderSize,
					SizeRemaining: 0,
				},
				{
					Size:          minimumStorageFolderSize * 4,
					SizeRemaining: 1,
				},
			},
			-1,
		},
	}
	for i, trial := range trials {
		sf, index := emptiestStorageFolder(trial.folders)
		if index != trial.emptiestIndex {
			t.Error("trial", i, "index mismatch")
		}
		if index > 0 && sf != trial.folders[index] {
			t.Error("trial", i, "folder mismatch")
		}
		if index < 0 && sf != nil {
			t.Error("non-nil storage folder returned but there was no winner")
		}
	}
}

// TestRepeatStorageFolderPath checks that the host correctly rejects a storage
// folder if there is already a storage folder linked to the same path.
func TestRepeatStorageFolderPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	smt, err := newStorageManagerTester("TestRepeatStorageFolderPath")
	if err != nil {
		t.Fatal(err)
	}
	defer smt.Close()

	err = smt.sm.AddStorageFolder(smt.persistDir, minimumStorageFolderSize)
	if err != nil {
		t.Fatal(err)
	}
	err = smt.sm.AddStorageFolder(smt.persistDir, minimumStorageFolderSize)
	if err != errRepeatFolder {
		t.Fatal("expected errRepeatFolder, got", err)
	}
}
*/
