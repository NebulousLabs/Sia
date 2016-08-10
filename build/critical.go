package build

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Critical will print a message to os.Stderr unless DEBUG has been set, in
// which case panic will be called instead.
func Critical(v ...interface{}) {
	s := "Critical error: " + fmt.Sprintln(v...) + "Please submit a bug report here: https://github.com/NebulousLabs/Sia/issues\n"
	if DEBUG {
		panic(s)
	}
	os.Stderr.WriteString(s)
	debug.PrintStack()
}
