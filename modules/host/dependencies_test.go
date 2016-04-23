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
