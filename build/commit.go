package build

// GitRevision, GitBranch, and BuildTime get assigned via the Makefile when
// built.
var (
	GitRevision, GitBranch, BuildTime string
)
