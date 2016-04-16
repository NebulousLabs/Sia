package build

import (
	"fmt"
	"os"
)

// Critical will print a message to os.Stderr unless DEBUG has been set, in
// which case panic will be called instead.
func Critical(v ...interface{}) {
	s := fmt.Sprintln(v...)
	if DEBUG {
		panic(s)
	}
	os.Stderr.WriteString(s)
}
