package wallet

// These interfaces define the Wallet's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimum
// possible subset of the real dependency.
type (
	// dependencies defines all of the dependencies of the Host.
	dependencies interface {
		// disrupt can be inserted in the code as a way to inject problems,
		disrupt(string) bool
	}
)

type (
	// productionDependencies is an empty struct
	productionDependencies struct{}
)

// disrupt will always return false, but can be over-written during testing to
// trigger disruptions.
func (productionDependencies) disrupt(string) bool {
	return false
}
