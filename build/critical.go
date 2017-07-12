package build

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Critical should be called if a sanity check has failed, indicating developer
// error. Critical is called with an extended message guiding the user to the
// issue tracker on Github. If the program does not panic, the call stack for
// the running goroutine is printed to help determine the error.
func Critical(v ...interface{}) {
	s := "Critical error: " + fmt.Sprintln(v...) + "Please submit a bug report here: https://github.com/NebulousLabs/Sia/issues\n"
	if Release != "testing" {
		debug.PrintStack()
		os.Stderr.WriteString(s)
	}
	if DEBUG {
		panic(s)
	}
}

// Severe will print a message to os.Stderr. If DEBUG has been set panic will
// be called as well. Severe should be called in situations which indicate
// significant problems for the user (such as disk failure or random number
// generation failure), but where crashing is not strictly required to preserve
// integrity.
func Severe(v ...interface{}) {
	s := "Severe error: " + fmt.Sprintln(v...)
	if Release != "testing" {
		debug.PrintStack()
		os.Stderr.WriteString(s)
	}
	if DEBUG {
		panic(s)
	}
}
