package persist

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/build"
)

// Logger is a wrapper for the standard library logger that enforces logging
// with the Sia-standard settings. It also supports a Close method, which
// attempts to close the underlying io.Writer.
type Logger struct {
	*log.Logger
	w io.Writer
}

// Critical logs a message with a CRITICAL prefix. If debug mode is enabled,
// it will also write the message to os.Stderr and panic.
func (l *Logger) Critical(v ...interface{}) {
	l.Print("CRITICAL:", fmt.Sprintln(v...))
	build.Critical(v...)
}

// Debug is equivalent to Logger.Print when build.DEBUG is true. Otherwise it
// is a no-op.
func (l *Logger) Debug(v ...interface{}) {
	if build.DEBUG {
		l.Print(v...)
	}
}

// Debugf is equivalent to Logger.Printf when build.DEBUG is true. Otherwise it
// is a no-op.
func (l *Logger) Debugf(format string, v ...interface{}) {
	if build.DEBUG {
		l.Printf(format, v...)
	}
}

// Debugln is equivalent to Logger.Println when build.DEBUG is true. Otherwise
// it is a no-op.
func (l *Logger) Debugln(v ...interface{}) {
	if build.DEBUG {
		l.Println(v...)
	}
}

// Close logs a shutdown message and closes the Logger's underlying io.Writer,
// if it is also an io.Closer.
func (l *Logger) Close() error {
	l.Println("SHUTDOWN: Logging has terminated.")
	if c, ok := l.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// NewLogger returns a logger that can be closed. Calls should not be made to
// the logger after 'Close' has been called.
func NewLogger(w io.Writer) *Logger {
	l := log.New(w, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC)
	l.Println("STARTUP: Logging has started.")
	return &Logger{l, w}
}

// closeableFile wraps an os.File to perform sanity checks on its Write and
// Close methods. When the checks are enabled, calls to Write or Close will
// panic if they are called after the file has already been closed.
type closeableFile struct {
	*os.File
	closed bool
	mu     sync.RWMutex
}

// Close closes the file and sets the closed flag.
func (cf *closeableFile) Close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	// Sanity check - close should not have been called yet.
	if cf.closed {
		build.Critical("cannot close the file; already closed")
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
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	// Sanity check - close should not have been called yet.
	if cf.closed {
		build.Critical("cannot write to the file after it has been closed")
	}
	return cf.File.Write(b)
}

// NewFileLogger returns a logger that logs to logFilename. The file is opened
// in append mode, and created if it does not exist.
func NewFileLogger(logFilename string) (*Logger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	cf := &closeableFile{File: logFile}
	return NewLogger(cf), nil
}
