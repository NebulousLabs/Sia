package main

import (
	"time"
)

const (
	// OutputRefreshRate is the rate at which siac will update something like a
	// progress meter when displaying a continuous action like a download.
	OutputRefreshRate = time.Millisecond * 250

	// RenterDownloadTimeout is the amount of time that needs to elapse before
	// the download command gives up on finding a download in the download list.
	RenterDownloadTimeout = time.Minute
)
