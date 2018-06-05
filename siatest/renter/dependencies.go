package renter

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/siatest"
)

// dependencyBlockScan blocks the scan progress of the hostdb until Scan is
// called on the dependency.
type dependencyBlockScan struct {
	modules.ProductionDependencies
	closed bool
	c      chan struct{}
}

// Disrupt will block the scan progress of the hostdb. The scan can be started
// by calling Scan on the dependency.
func (d *dependencyBlockScan) Disrupt(s string) bool {
	if d.c == nil {
		d.c = make(chan struct{})
	}
	if s == "BlockScan" {
		<-d.c
	}
	return false
}

// Scan resumes the blocked scan.
func (d *dependencyBlockScan) Scan() {
	if d.closed {
		return
	}
	close(d.c)
	d.closed = true
}

// newDependencyInterruptDownloadBeforeSendingRevision creates a new dependency
// that interrupts the download on the renter side before sending the signed
// revision to the host.
func newDependencyInterruptDownloadBeforeSendingRevision() *siatest.DependencyInterruptOnceOnKeyword {
	return siatest.NewDependencyInterruptOnceOnKeyword("InterruptDownloadBeforeSendingRevision")
}

// newDependencyInterruptDownloadAfterSendingRevision creates a new dependency
// thta interrupts the download on the renter side right after receiving the
// signed revision from the host.
func newDependencyInterruptDownloadAfterSendingRevision() *siatest.DependencyInterruptOnceOnKeyword {
	return siatest.NewDependencyInterruptOnceOnKeyword("InterruptDownloadAfterSendingRevision")
}

// newDependencyInterruptUploadBeforeSendingRevision creates a new dependency
// that interrupts the upload on the renter side before sending the signed
// revision to the host.
func newDependencyInterruptUploadBeforeSendingRevision() *siatest.DependencyInterruptOnceOnKeyword {
	return siatest.NewDependencyInterruptOnceOnKeyword("InterruptUploadBeforeSendingRevision")
}

// newDependencyInterruptUploadAfterSendingRevision creates a new dependency
// thta interrupts the upload on the renter side right after receiving the
// signed revision from the host.
func newDependencyInterruptUploadAfterSendingRevision() *siatest.DependencyInterruptOnceOnKeyword {
	return siatest.NewDependencyInterruptOnceOnKeyword("InterruptUploadAfterSendingRevision")
}
