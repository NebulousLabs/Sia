package renter

import "github.com/NebulousLabs/Sia/siatest"

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
