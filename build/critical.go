package build

import (
	"os"
)

// Critical will print a message to os.Stderr unless DEBUG has been set, in
// which case panic will be called instead.
func Critical(s string) {
	os.Stderr.WriteString(s)
	if DEBUG {
		panic(s)
	}
}
