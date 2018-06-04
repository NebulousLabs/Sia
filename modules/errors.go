package modules

import "github.com/NebulousLabs/errors"

// ErrHostFault is an error that is usually extended to indicate that an error
// is the host's fault.
var ErrHostFault = errors.New("")

// IsHostsFault indicates if a returned error is the host's fault.
func IsHostsFault(err error) bool {
	return errors.Contains(err, ErrHostFault)
}
