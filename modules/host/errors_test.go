package host

import (
	"errors"
	"testing"
)

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
	switch err2.(type) {
	case ErrorCommunication:
	default:
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
