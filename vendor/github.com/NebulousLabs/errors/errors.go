// Package errors is an extension of and drop in replacement for the standard
// library errors package. Multiple errors can be composed into a single error,
// and single errors can be extended with new errors or with context. You can
// also check whether any of the extensions or compositions contains a certain
// error, allowing for more flexible error handling for complex processes.
package errors

import (
	"errors"
	"os"
)

// Error satisfies the error interface, and additionally keeps track of all
// extensions and compositions that have happened to the underlying errors.
type Error struct {
	ErrSet []error
}

// Error returns the composed error string of the Error, clustering errors by
// their composition and extension.
func (r Error) Error() string {
	s := "["
	for i, err := range r.ErrSet {
		if i != 0 {
			s = s + "; "
		}
		s = s + err.Error()
	}
	return s + "]"
}

// Compose will compose all errors together into a single error, remembering
// each component error so that they can be checked for matches later using the
// `Contains` function.
//
// Any `nil` input errors will be ignored. If all input errors are `nil`, then
// `nil` will be returned.
func Compose(errs ...error) error {
	var r Error
	for _, err := range errs {
		// Handle nil errors.
		if err == nil {
			continue
		}
		r.ErrSet = append(r.ErrSet, err)
	}
	if len(r.ErrSet) == 0 {
		return nil
	}
	return r
}

// Contains will check whether the base error contains the cmp error. If the
// base err is a Error, then it will check whether there is a match on any of
// the underlying errors.
func Contains(base, cmp error) bool {
	// Check for the easy edge cases.
	if cmp == nil || base == nil {
		return false
	}
	if base == cmp {
		return true
	}

	switch v := base.(type) {
	case Error:
		for _, err := range v.ErrSet {
			if Contains(err, cmp) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Extend will extend the first error with the second error, remembering each
// component error so that they can be checked for matches later using the
// `Contains` function.
//
// Any `nil` input will be ignored. If both inputs are `nil, then `nil` will be
// returned.
func Extend(err, extension error) error {
	// Check for nil edge cases. If both are nil, nil will be returned.
	if err == nil {
		return extension
	}
	if extension == nil {
		return err
	}

	var r Error
	// Check the original error for richness.
	switch v := err.(type) {
	case Error:
		r = v
	default:
		r.ErrSet = []error{v}
	}

	// Check the extension error for richness.
	switch v := extension.(type) {
	case Error:
		r.ErrSet = append(v.ErrSet, r.ErrSet...)
	default:
		r.ErrSet = append([]error{v}, r.ErrSet...)
	}

	// Return nil if the result has no underlying errors.
	if len(r.ErrSet) == 0 {
		return nil
	}
	return r
}

// IsOSNotExist returns true if any of the errors in the underlying composition
// return true for os.IsNotExist.
func IsOSNotExist(err error) bool {
	if err == nil {
		return false
	}

	switch v := err.(type) {
	case Error:
		for _, err := range v.ErrSet {
			if IsOSNotExist(err) {
				return true
			}
		}
		return false
	default:
		return os.IsNotExist(err)
	}
}

// New is a passthrough to the stdlib errors package, allowing
// NebulousLabs/errors to be a drop in replacement for the standard library
// errors package.
func New(s string) error {
	return errors.New(s)
}
