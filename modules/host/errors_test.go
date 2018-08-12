package host

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// countFileLines is a helper function that will count the number of lines in a
// file, based on the number of '\n' characters. countFileLines will load the
// file into memory using ioutil.ReadAll.
//
// countFileLines will ignore all lines with the string 'DEBUG' in it.
func countFileLines(filepath string) (uint64, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(file)
	lines := uint64(0)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "[DEBUG]") {
			lines++
		}
	}
	return lines, nil
}

// TestComposeErrors checks that composeErrors is correctly composing errors
// and handling edge cases.
func TestComposeErrors(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	trials := []struct {
		inputErrors           []error
		nilReturn             bool
		expectedComposedError string
	}{
		{
			nil,
			true,
			"",
		},
		{
			make([]error, 0),
			true,
			"",
		},
		{
			[]error{errors.New("single error")},
			false,
			"single error",
		},
		{
			[]error{
				errors.New("first error"),
				errors.New("second error"),
			},
			false,
			"first error; second error",
		},
		{
			[]error{
				errors.New("first error"),
				errors.New("second error"),
				errors.New("third error"),
			},
			false,
			"first error; second error; third error",
		},
		{
			[]error{
				nil,
				errors.New("second error"),
				errors.New("third error"),
			},
			false,
			"second error; third error",
		},
		{
			[]error{
				errors.New("first error"),
				nil,
				nil,
			},
			false,
			"first error",
		},
		{
			[]error{
				nil,
				nil,
				nil,
			},
			true,
			"",
		},
	}
	for _, trial := range trials {
		err := composeErrors(trial.inputErrors...)
		if trial.nilReturn {
			if err != nil {
				t.Error("composeError failed a test, expecting nil, got", err)
			}
		} else {
			if err == nil {
				t.Error("not expecting a nil error when doing composition")
			}
			if err.Error() != trial.expectedComposedError {
				t.Error("composeError failed a test, expecting", trial.expectedComposedError, "got", err.Error())
			}
		}
	}
}

// TestExtendErr checks that extendErr works as described - preserving the
// error type within the package and adding a string. Also returning nil if the
// input error is nil.
func TestExtendErr(t *testing.T) {
	// Try extending a nil error.
	var err error
	err2 := extendErr("extend: ", err)
	if err2 != nil {
		t.Error("providing a nil error to extendErr does not return nil")
	}

	// Try extending a normal error.
	err = errors.New("extend me")
	err2 = extendErr("extend: ", err)
	if err2.Error() != "extend: extend me" {
		t.Error("normal error not extended correctly")
	}

	// Try extending ErrorCommunication.
	err = ErrorCommunication("err")
	err2 = extendErr("extend: ", err)
	if err2.Error() != "communication error: extend: err" {
		t.Error("extending ErrorCommunication did not occur correctly:", err2.Error())
	}
	if _, ok := err2.(ErrorCommunication); !ok {
		t.Error("extended error did not preserve error type")
	}

	// Try extending ErrorConnection.
	err = ErrorConnection("err")
	err2 = extendErr("extend: ", err)
	if err2.Error() != "connection error: extend: err" {
		t.Error("extending ErrorConnection did not occur correctly:", err2.Error())
	}
	switch err2.(type) {
	case ErrorConnection:
	default:
		t.Error("extended error did not preserve error type")
	}

	// Try extending ErrorConsensus.
	err = ErrorConsensus("err")
	err2 = extendErr("extend: ", err)
	if err2.Error() != "consensus error: extend: err" {
		t.Error("extending ErrorConsensus did not occur correctly:", err2.Error())
	}
	switch err2.(type) {
	case ErrorConsensus:
	default:
		t.Error("extended error did not preserve error type")
	}

	// Try extending ErrorInternal.
	err = ErrorInternal("err")
	err2 = extendErr("extend: ", err)
	if err2.Error() != "internal error: extend: err" {
		t.Error("extending ErrorInternal did not occur correctly:", err2.Error())
	}
	switch err2.(type) {
	case ErrorInternal:
	default:
		t.Error("extended error did not preserve error type")
	}
}

// TestManagedLogError will check that errors are being logged correctly based
// on the logAllLimit, the probabilities, and the logFewLimit.
func TestManagedLogError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestManagedLogError")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()
	logFilepath := filepath.Join(ht.persistDir, modules.HostDir, logFile)

	// Count the number of lines in the log file.
	baseLines, err := countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}

	// Log 'logAllLimit' for ErrorCommunication.
	for i := uint64(0); i < logAllLimit; i++ {
		ht.host.managedLogError(ErrorCommunication("comm error"))
	}
	logLines, err := countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines != baseLines+logAllLimit {
		t.Error("does not seem that all communication errors were logged")
	}
	baseLines = logLines

	// Log 'logAllLimit' for ErrorConnection.
	for i := uint64(0); i < logAllLimit; i++ {
		ht.host.managedLogError(ErrorConnection("conn error"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines != baseLines+logAllLimit {
		t.Error("does not seem that all connection errors were logged")
	}
	baseLines = logLines

	// Log 'logAllLimit' for ErrorConsensus.
	for i := uint64(0); i < logAllLimit; i++ {
		ht.host.managedLogError(ErrorConsensus("consensus error"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines != baseLines+logAllLimit {
		t.Error("does not seem that all consensus errors were logged")
	}
	baseLines = logLines

	// Log 'logAllLimit' for ErrorInternal.
	for i := uint64(0); i < logAllLimit; i++ {
		ht.host.managedLogError(ErrorInternal("internal error"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines != baseLines+logAllLimit {
		t.Error("does not seem that all internal errors were logged")
	}
	baseLines = logLines

	// Log 'logAllLimit' for normal errors.
	for i := uint64(0); i < logAllLimit; i++ {
		ht.host.managedLogError(errors.New("normal error"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines != baseLines+logAllLimit {
		t.Error("does not seem that all normal errors were logged", logLines, baseLines, logAllLimit)
	}
	baseLines = logLines

	// Log enough ErrorInternal errors to bring ErrorInternal close, but not
	// all the way, to the 'logFewLimit'.
	remaining := logFewLimit - logAllLimit
	logsNeeded := remaining * errorInternalProbability
	for i := uint64(0); i < logsNeeded/3; i++ {
		ht.host.managedLogError(ErrorInternal("internal err"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines < baseLines+remaining/6 || logLines > baseLines+remaining {
		t.Error("probabilistic logging is not logging with the correct probability:", logLines, baseLines, remaining)
	}
	// Log enough ErrorInternal errors to bring it all the way to
	// 'logFewLimit'.
	for i := uint64(0); i < logsNeeded*5; i++ {
		ht.host.managedLogError(ErrorInternal("internal err"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines < baseLines+remaining || logLines > baseLines+logsNeeded*2 {
		t.Error("probabilisitic logging is not clamping correctly:", baseLines, logLines, logsNeeded)
	}
	baseLines = logLines

	// Log enough ErrorCommunication errors to bring ErrorCommunication close, but not
	// all the way, to the 'logFewLimit'.
	remaining = logFewLimit - logAllLimit
	logsNeeded = remaining * errorCommunicationProbability
	for i := uint64(0); i < logsNeeded/3; i++ {
		ht.host.managedLogError(ErrorCommunication("comm err"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines < baseLines+remaining/6 || logLines > baseLines+remaining {
		t.Error("probabilistic logging is not logging with the correct probability:", baseLines, logLines, logsNeeded, remaining)
	}
	// Log enough ErrorCommunication errors to bring it all the way to
	// 'logFewLimit'.
	for i := uint64(0); i < logsNeeded*5; i++ {
		ht.host.managedLogError(ErrorCommunication("comm err"))
	}
	logLines, err = countFileLines(logFilepath)
	if err != nil {
		t.Fatal(err)
	}
	if logLines < baseLines+remaining || logLines > baseLines+logsNeeded*2 {
		t.Error("probabilisitic logging is not clamping correctly:", baseLines, logLines, logsNeeded, remaining)
	}
}
