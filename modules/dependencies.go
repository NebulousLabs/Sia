package modules

// These interfaces define the modle's dependencies. Mocking implementation
// complexity can be reduced by defining each dependency as the minimum
// possible subset of the real dependency.
type (
	// Dependencies defines all of the dependencies of the module.
	Dependencies interface {
		// disrupt can be inserted in the code as a way to inject problems,
		Disrupt(string) bool
	}
)
