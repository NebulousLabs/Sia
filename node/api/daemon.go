package api

// DaemonVersionGet contains information about the running daemon's version.
type DaemonVersionGet struct {
	Version     string
	GitRevision string
	GitBranch   string
	BuildTime   string
}
