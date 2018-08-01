package persist

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestLogger checks that the basic functions of the file logger work as
// designed.
func TestLogger(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a folder for the log file.
	testdir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Create the logger.
	logFilename := filepath.Join(testdir, "test.log")
	fl, err := NewFileLogger(logFilename)
	if err != nil {
		t.Fatal(err)
	}

	// Write an example statement, and then close the logger.
	fl.Println("TEST: this should get written to the logfile")
	err = fl.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Check that data was written to the log file. There should be three
	// lines, one for startup, the example line, and one to close the logger.
	expectedSubstring := []string{"STARTUP", "TEST", "SHUTDOWN", ""} // file ends with a newline
	fileData, err := ioutil.ReadFile(logFilename)
	if err != nil {
		t.Fatal(err)
	}
	fileLines := strings.Split(string(fileData), "\n")
	for i, line := range fileLines {
		if !strings.Contains(string(line), expectedSubstring[i]) {
			t.Error("did not find the expected message in the logger")
		}
	}
	if len(fileLines) != 4 { // file ends with a newline
		t.Error("logger did not create the correct number of lines:", len(fileLines))
	}
}

// TestLoggerCritical prints a critical message from the logger.
func TestLoggerCritical(t *testing.T) {
	// Create a folder for the log file.
	testdir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	// Create the logger.
	logFilename := filepath.Join(testdir, "test.log")
	fl, err := NewFileLogger(logFilename)
	if err != nil {
		t.Fatal(err)
	}

	// Write a catch for a panic that should trigger when logger.Critical is
	// called.
	defer func() {
		r := recover()
		if r == nil {
			t.Error("critical message was not thrown in a panic")
		}

		// Close the file logger to clean up the test.
		err = fl.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	fl.Critical("a critical message")
}
