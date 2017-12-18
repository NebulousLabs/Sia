package wallet

type (
	// dependencyAcceptTxnSetFailed is a dependency used to cause a call to
	// SendSiacoins and SendSiacoinsMulti to fail before AcceptTransactionSet
	// is called
	dependencySendSiacoinsInterrupted struct {
		productionDependencies
		f bool
	}
)

// disrupt will return true if fail was called and the correct string value is
// provided
func (d dependencySendSiacoinsInterrupted) disrupt(s string) bool {
	if s == "SendSiacoinsInterrupted" {
		return true
	}
	return false
}
