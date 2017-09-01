package renter

// These interfaces define the Renter's dependencies. Using the smallest
// interface possible makes it easier to mock these dependencies in testing.
type (
	dependencies interface {
		disrupt(string) bool
	}
)

type prodDependencies struct{}

func (prodDependencies) disrupt(string) bool { return false }
