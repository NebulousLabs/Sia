package build

import (
	"errors"
	"strings"
)

// ComposeErrors will take multiple errors and compose them into a single
// errors with a longer message. Any nil errors used as inputs will be stripped
// out, and if there are zero non-nil inputs then 'nil' will be returned.
//
// The original types of the errors is not preserved at all.
func ComposeErrors(errs ...error) error {
	// Strip out any nil errors.
	var errStrings []string
	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}

	// Return nil if there are no non-nil errors in the input.
	if len(errStrings) <= 0 {
		return nil
	}

	// Combine all of the non-nil errors into one larger return value.
	return errors.New(strings.Join(errStrings, "; "))
}

// ExtendErr will return a new error which extends the input error with a
// string. If the input error is nil, then 'nil' will be returned, discarding
// the input string.
func ExtendErr(s string, err error) error {
	if err == nil {
		return nil
	}
	return errors.New(s + ": " + err.Error())
}

// JoinErrors concatenates the elements of errs to create a single error. The
// separator string sep is placed between elements in the resulting error. Nil
// errors are skipped. If errs is empty or only contains nil elements,
// JoinErrors returns nil.
func JoinErrors(errs []error, sep string) error {
	var strs []string
	for _, err := range errs {
		if err != nil {
			strs = append(strs, err.Error())
		}
	}
	if len(strs) > 0 {
		return errors.New(strings.Join(strs, sep))
	}
	return nil
}
