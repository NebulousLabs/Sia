package renter

import (
	"sync"

	"github.com/NebulousLabs/Sia/modules"
)

type (
	// dependencyRenterInterruptDownloadBeforeCommit interrupts a download on the
	// Renter side after the negotiation with the host but before committing
	// the changes.
	dependencyRenterInterruptDownloadBeforeCommit struct {
		f bool // indicates if the next download should fail
		modules.ProductionDependencies
		mu sync.Mutex
	}
	// dependencyRenterInterruptUploadBeforeCommit interrupts an upload on the
	// Renter side after the negotiation with the host but before committing
	// the changes.
	dependencyRenterInterruptUploadBeforeCommit struct {
		f bool // indicates if the next upload should fail
		modules.ProductionDependencies
		mu sync.Mutex
	}
)

// Disrupt returns true if the correct string is provided and if the flag was
// set to true by calling fail on the dependency beforehand. After simulating a
// crash the flag will be set to false and fail has to be called again for
// another disruption.
func (d *dependencyRenterInterruptDownloadBeforeCommit) Disrupt(s string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f && s == "DownloadCrashBeforeCommit" {
		d.f = false
		return true
	}
	return false
}

// Disrupt returns true if the correct string is provided and if the flag was
// set to true by calling fail on the dependency beforehand. After simulating a
// crash the flag will be set to false and fail has to be called again for
// another disruption.
func (d *dependencyRenterInterruptUploadBeforeCommit) Disrupt(s string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f && s == "UploadCrashBeforeCommit" {
		d.f = false
		return true
	}
	return false
}

// fail causes the next call to Disrupt to return true if the correct string is
// provided.
func (d *dependencyRenterInterruptDownloadBeforeCommit) fail() {
	d.mu.Lock()
	d.f = true
	d.mu.Unlock()
}

// fail causes the next call to Disrupt to return true if the correct string is
// provided.
func (d *dependencyRenterInterruptUploadBeforeCommit) fail() {
	d.mu.Lock()
	d.f = true
	d.mu.Unlock()
}
