package build

// GitRevision and BuildTime get assigned via the Makefile when built.
var (
	// GitRevision is the git commit hash used when built
	GitRevision string
	// BuildTime is the date and time the build was completed
	BuildTime string
)
