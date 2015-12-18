package persist

import (
	"log"
	"os"
)

// Logger is a wrapper for the standard library logger that enforces logging
// into a file with the Sia-standard settings.
type Logger struct {
	*log.Logger
	logFile *os.File
}

// NewLogger returns a logger that can be closed.
func NewLogger(logFilename string) (*Logger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC)
	logger.Println("STARTUP: Logging has started.")
	return &Logger{Logger: logger, logFile: logFile}, nil
}

// Close terminates the Logger.
func (l *Logger) Close() error {
	l.Println("SHUTDOWN: Logging has terminated.")
	return l.logFile.Close()
}
