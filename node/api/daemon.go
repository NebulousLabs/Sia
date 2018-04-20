package api

// DaemonVersionGet contains information about the running daemon's version.
type DaemonVersionGet struct {
	Version     string
	GitRevision string
	BuildTime   string
}

// DaemonUpdateGet contains information about a potential available update for
// the daemon.
type DaemonUpdateGet struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
}
