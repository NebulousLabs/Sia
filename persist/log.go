package persist

import (
	"fmt"
	"log"
	"os"

	"github.com/NebulousLabs/Sia/build"
)

// closeableFile wraps an os.File to perform sanity checks on its Write and
// Close methods. When the checks are enable, calls to Write or Close will
// panic if they are called after the file has already been closed.
type closeableFile struct {
	*os.File
	closed bool
}

// Close closes the file and sets the closed flag.
func (cf *closeableFile) Close() error {
	// Sanity check - close should not have been called yet.
	if build.DEBUG && cf.closed {
		panic("cannot close the file; already closed")
	}
	// Ensure that all data has actually hit the disk.
	if err := cf.Sync(); err != nil {
		return err
	}
	cf.closed = true
	return cf.File.Close()
}

// Write takes the input data and writes it to the file.
func (cf *closeableFile) Write(b []byte) (int, error) {
	// Sanity check - close should not have been called yet.
	if build.DEBUG && cf.closed {
		panic("cannot write to the file after it has been closed")
	}
	return cf.File.Write(b)
}

// Logger is a wrapper for the standard library logger that enforces logging
// into a file with the Sia-standard settings.
type Logger struct {
	*log.Logger
	file *closeableFile
}

// NewLogger returns a logger that can be closed. Calls should not be made to
// the logger after 'Close' has been called.
func NewLogger(logFilename string) (*Logger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	cf := &closeableFile{File: logFile}
	l := log.New(cf, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC)
	l.Println("STARTUP: Logging has started.")
	return &Logger{Logger: l, file: cf}, nil
}

// Close terminates the Logger.
func (l *Logger) Close() error {
	l.Println("SHUTDOWN: Logging has terminated.")
	return l.file.Close()
}

// Critical will panic if debug mode is activated, and will log the statement
// otherwise.
func (l *Logger) Critical(v ...interface{}) {
	s := fmt.Sprintln(v...)
	os.Stderr.WriteString("Severe Error: " + s)
	l.Println("CRITICAL:", s)
	if build.DEBUG {
		panic(s)
	}
}
