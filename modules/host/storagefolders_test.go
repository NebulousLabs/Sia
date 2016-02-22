package host

import (
	"testing"
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
