package build

import (
	"errors"
	"testing"
)

// TestJoinErrors tests that JoinErrors only returns non-nil when there are
// non-nil elements in errs. And tests that the returned error's string the
// concatenation of all the strings of the elements in errs, in order and
// separated by sep.
func TestJoinErrors(t *testing.T) {
	tests := []struct {
		errs       []error
		sep        string
		wantNil    bool
		errStrWant string
	}{
		// Test that JoinErrors returns nil when errs is nil.
		{
			wantNil: true,
		},
		// Test that JoinErrors returns nil when errs is an empty slice.
		{
			errs:    []error{},
			wantNil: true,
		},
		// Test that JoinErrors returns nil when errs has only nil elements.
		{
			errs:    []error{nil},
			wantNil: true,
		},
		{
			errs:    []error{nil, nil, nil},
			wantNil: true,
		},
		// Test that JoinErrors returns non-nil with the expected string when errs has only one non-nil element.
		{
			errs:       []error{errors.New("foo")},
			sep:        ";",
			errStrWant: "foo",
		},
		// Test that JoinErrors returns non-nil with the expected string when errs has multiple non-nil elements.
		{
			errs:       []error{errors.New("foo"), errors.New("bar"), errors.New("baz")},
			sep:        ";",
			errStrWant: "foo;bar;baz",
		},
		// Test that nil errors are ignored.
		{
			errs:       []error{nil, errors.New("foo"), nil, nil, nil, errors.New("bar"), errors.New("baz"), nil, nil, nil},
			sep:        ";",
			errStrWant: "foo;bar;baz",
		},
	}
	for _, tt := range tests {
		err := JoinErrors(tt.errs, tt.sep)
		if tt.wantNil && err != nil {
			t.Errorf("expected nil error, got '%v'", err)
		} else if err != nil && err.Error() != tt.errStrWant {
			t.Errorf("expected '%v', got '%v'", tt.errStrWant, err)
		}
	}
}
