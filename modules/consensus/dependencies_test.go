package consensus

import "github.com/NebulousLabs/Sia/modules"

type (
	// dependencySleepAfterInitializeSubscribe is a dependency used to make the consensusSet sleep for a few seconds after calling managedInitializeSubscribe.
	dependencySleepAfterInitializeSubscribe struct {
		modules.ProductionDependencies
		f bool // indicates if the next call should fail
	}
)

// Disrupt will return true if fail was called and the correct string value is
// provided. It also resets f back to false. This means fail has to be called
// once for each Send that should fail.
func (d *dependencySleepAfterInitializeSubscribe) Disrupt(s string) bool {
	if d.f && s == "SleepAfterInitializeSubscribe" {
		d.f = false
		return true
	}
	return false
}

// enable causes the next "SleepAfterInitializeSubscribe" - disrupt of this
// dependency to return true
func (d *dependencySleepAfterInitializeSubscribe) enable() {
	d.f = true
}
