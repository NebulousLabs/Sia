package persist

import (
	"log"
	"os"

	"github.com/NebulousLabs/Sia/build"
)

// logFileWrapper wraps a log file to perform sanity checks on the Write and
// Close functions. An error will be returned if Close is called multiple times
// or if Write is called after Close has been called.
type logFileWrapper struct {
	closed bool // Flag is set after Close is called.
	file   *os.File
}

// newLogFileWrapper returns a logFileWrapper that has been initialized with
// the input file.
func newLogFileWrapper(f *os.File) *logFileWrapper {
	return &logFileWrapper{file: f}
}

// Close closes the log file wrapper.
func (lfw *logFileWrapper) Close() error {
	// Sanity check - close should not have been called yet.
	if build.DEBUG && lfw.closed {
		panic("cannot close the logger after it has been closed")
	}
	lfw.closed = true
	return lfw.file.Close()
}

// Write takes the input data and writes it to the file.
func (lfw *logFileWrapper) Write(b []byte) (int, error) {
	// Sanity check - close should not have been called yet.
	if build.DEBUG && lfw.closed {
		panic("cannot write to the logger after it has been closed")
	}
	return lfw.file.Write(b)
}

// Logger is a wrapper for the standard library logger that enforces logging
// into a file with the Sia-standard settings.
type Logger struct {
	*log.Logger
	logFileWrapper *logFileWrapper
}

// NewLogger returns a logger that can be closed. Calls should not be made to
// the logger after 'Close' has been called.
func NewLogger(logFilename string) (*Logger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	lfw := newLogFileWrapper(logFile)
	logger := log.New(lfw, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC)
	logger.Println("STARTUP: Logging has started.")
	return &Logger{Logger: logger, logFileWrapper: lfw}, nil
}

// Close terminates the Logger.
func (l *Logger) Close() error {
	l.Println("SHUTDOWN: Logging has terminated.")
	return l.logFileWrapper.Close()
}
