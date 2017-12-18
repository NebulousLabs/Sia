package wallet

type (
	// dependencyAcceptTxnSetFailed is a dependency used to cause a call to
	// SendSiacoins and SendSiacoinsMulti to fail before AcceptTransactionSet
	// is called
	dependencySendSiacoinsInterrupted struct {
		ProductionDependencies
		f bool // indicates if the next call should fail
	}
)

// disrupt will return true if fail was called and the correct string value is
// provided
func (d *dependencySendSiacoinsInterrupted) disrupt(s string) bool {
	if d.f && s == "SendSiacoinsInterrupted" {
		d.f = false
		return true
	}
	return false
}

// fail causes the next SendsiacoinsInterrupted disrupt to return true
func (d *dependencySendSiacoinsInterrupted) fail() {
	d.f = true
}
