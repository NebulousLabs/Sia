package main

/*
import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/sia/components"
)

func testUploadFile(t *testing.T, c *Core) {
	if testing.Short() {
		return
	}
	// Check that hostDB has at least one entry.
	if c.hostDB.Size() < 1 {
		t.Fatal("Hostdb needs at least 1 host to perform testUploadFile")
	}

	// Get the initial volume of files in the renter dataset.
	rentInfo, err := c.renter.RentInfo()
	if err != nil {
		t.Fatal(err)
	}
	fileCount := len(rentInfo.Files)

	// Have the renter negotiate a contract with the host in the hostDB.
	randData := make([]byte, 216)
	rand.Read(randData)
	err = c.renter.RentSmallFile(components.RentSmallFileParameters{
		FullFile:    randData,
		Nickname:    "one",
		TotalPieces: 1,
	})
	if err != nil {
		t.Error(err)
	}

	time.Sleep(10 * time.Second)

	// Check that the file has been added to the renter fileset.
	rentInfo, err = c.renter.RentInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(rentInfo.Files) != fileCount+1 {
		t.Error("Renter fileset did not increase after uploading")
	}

	// Check that the file has been added to the host.
	if c.host.NumContracts() == 0 {
		t.Error("Host is not reporting a new contract.")
	}

	// Check that hostDB has at least one entry.
	if c.hostDB.Size() < 1 {
		t.Fatal("Hostdb got pruned while trying to make a contract?")
	}

	// Check that the file can be downloaded.
	err = c.renter.Download("one", "renterDownload")
	if err != nil {
		t.Error(err)
	}
}
*/
