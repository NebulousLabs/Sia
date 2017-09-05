package modules

// These interfaces defines dependencies that can be used to disrupt the execution
// of a module
type (
	Dependencies interface {
		Disrupt(string) bool
	}
)
