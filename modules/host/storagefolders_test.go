package host

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
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
	ht, err := newHostTester("TestAddStorageFolderUIDCollisions")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the environment requirements for the test have been met.
	if storageFolderUIDSize != 1 {
		t.Fatal("For this test, the host must be using storage folder UIDs that are 1 byte")
	}
	if maximumStorageFolders < 100 {
		t.Fatal("For this test, the host must be allowed to have at least 100 storage folders")
	}

	// Create 100 storage folders, and check that there are no collisions
	// between any of them. Because the UID is only using 1 byte, once there
	// are more than 64 there will be at least 1/4 chance of a collision for
	// each randomly selected UID. Running into collisions is virtually
	// guaranteed, and running into repeated collisions (where two UIDs
	// consecutively collide with existing UIDs) are highly likely.
	for i := 0; i < 100; i++ {
		ht.host.AddStorageFolder(ht.host.persistDir, minimumStorageFolderSize)
	}
	// Check that there are no collisions.
	uidMap := make(map[uint8]struct{})
	for _, sf := range ht.host.storageFolders {
		_, exists := uidMap[uint8(sf.UID[0])]
		if exists {
			t.Error("Collision")
		}
		uidMap[uint8(sf.UID[0])] = struct{}{}
	}

	// Try again, this time removing a random storage folder and then adding
	// another one repeatedly - enough times to exceed the 256 possible folder
	// UIDs that be chosen in the testing environment.
	for i := 0; i < 300; i++ {
		// Repalce the very first storage folder.
		ht.host.RemoveStorageFolder(0, false)
		ht.host.AddStorageFolder(ht.host.persistDir, minimumStorageFolderSize)

		// Replace a random storage folder.
		n, err := crypto.RandIntn(100)
		if err != nil {
			t.Fatal(err)
		}
		ht.host.RemoveStorageFolder(n, false)
		ht.host.AddStorageFolder(ht.host.persistDir, minimumStorageFolderSize)
	}
	uidMap = make(map[uint8]struct{})
	for _, sf := range ht.host.storageFolders {
		_, exists := uidMap[uint8(sf.UID[0])]
		if exists {
			t.Error("Collision")
		}
		uidMap[uint8(sf.UID[0])] = struct{}{}
	}
}
