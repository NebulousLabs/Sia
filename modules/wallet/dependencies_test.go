package wallet

import "gitlab.com/NebulousLabs/Sia/modules"

type (
	// dependencyAcceptTxnSetFailed is a dependency used to cause a call to
	// SendSiacoins and SendSiacoinsMulti to fail before AcceptTransactionSet
	// is called
	dependencySendSiacoinsInterrupted struct {
		modules.ProductionDependencies
		f bool // indicates if the next call should fail
	}

	// dependencyDefragInterrupted is a dependency used to cause a defrag to
	// fail before AcceptTransactionSet is called
	dependencyDefragInterrupted struct {
		modules.ProductionDependencies
		f bool // indicates if the next call should fail
	}
)

// Disrupt will return true if fail was called and the correct string value is
// provided. It also resets f back to false. This means fail has to be called
// once for each Send that should fail.
func (d *dependencySendSiacoinsInterrupted) Disrupt(s string) bool {
	if d.f && s == "SendSiacoinsInterrupted" {
		d.f = false
		return true
	}
	return false
}

// fail causes the next SendSiacoinsInterrupted disrupt to return true
func (d *dependencySendSiacoinsInterrupted) fail() {
	d.f = true
}

// Disrupt will return true if fail was called and the correct string value is
// provided. It also resets f back to false. This means fail has to be called
// once for each Defrag that should fail.
func (d *dependencyDefragInterrupted) Disrupt(s string) bool {
	if d.f && s == "DefragInterrupted" {
		d.f = false
		return true
	}
	return false
}

// fail causes the next DefragInterrupted disrupt to return true
func (d *dependencyDefragInterrupted) fail() {
	d.f = true
}
