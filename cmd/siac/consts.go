package main

import (
	"time"
)

// OutputRefreshRate is the rate at which siac will update something like a
// progress meter when displaying a continuous action like a download to a user.
const OutputRefreshRate = time.Millisecond * 250
