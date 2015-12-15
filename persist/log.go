package persist

import (
	"log"
	"os"
)

// FileLogger is a wrapper for the standard library logger that enforces logging
// into a file with the Sia-standard settings.
type FileLogger struct {
	*log.Logger
	logFile *os.File
}

// CreateLogger returns a logger that can be closed.
func CreateFileLogger(logFilename string) (*FileLogger, error) {
	logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile|log.LUTC)
	logger.Println("STARTUP: Logging has started.")
	return &FileLogger{Logger: logger, logFile: logFile}, nil
}

// Close terminates the Logger.
func (l *FileLogger) Close() error {
	return l.logFile.Close()
}
