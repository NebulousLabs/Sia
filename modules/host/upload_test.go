package host

import (
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// uploadTestFile uploads a file to the host from the tester's renter.
func (ht *hostTester) uploadFile(name string) error {
	// Check that renting is initialized properly.
	err := ht.initRenting()
	if err != nil {
		return err
	}

	// Create a file to upload to the host.
	filepath := filepath.Join(ht.persistDir, name+".testfile")
	data, err := crypto.RandBytes(1024)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, data, 0600)
	if err != nil {
		return err
	}

	// Have the renter upload to the host.
	fup := modules.FileUploadParams{
		Filename:    filepath,
		Nickname:    name, // TODO: setting the nickname to 'filepath' failed?
		Duration:    20,
		Renew:       false,
		ErasureCode: nil,
		PieceSize:   0,
	}
	err = ht.renter.Upload(fup)
	if err != nil {
		return err
	}

	// Wait until the upload has finished.
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 100)

		// Asynchronous processes in the host access obligations by id.
		ht.host.mu.Lock()
		lenOBID := len(ht.host.obligationsByID)
		ht.host.mu.Unlock()
		if lenOBID != 0 {
			break
		}
	}
	// Asynchronous processes in the host access obligations by id.
	ht.host.mu.Lock()
	lenOBID := len(ht.host.obligationsByID)
	ht.host.mu.Unlock()
	if lenOBID == 0 {
		return err
	}
	return nil
}

// TestRPCUPload attempts to upload a file to the host, adding coverage to the
// upload function.
func TestRPCUpload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestRPCUpload")
	if err != nil {
		t.Fatal(err)
	}
	err = ht.uploadFile("TestRPCUpload - 1")
	if err != nil {
		t.Fatal(err)
	}

	// TODO: Check that the anticipated revenue has increased.
}
