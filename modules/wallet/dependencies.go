package wallet

// These interfaces define the Wallet's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimum
// possible subset of the real dependency.
type (
	// Dependencies defines all of the dependencies of the Host.
	Dependencies interface {
		// disrupt can be inserted in the code as a way to inject problems,
		Disrupt(string) bool
	}
)

type (
	// ProductionDependencies is an empty struct
	ProductionDependencies struct{}
)

// Disrupt will always return false, but can be over-written during testing to
// trigger disruptions.
func (*ProductionDependencies) Disrupt(string) bool {
	return false
}
