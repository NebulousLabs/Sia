package build

import (
	"errors"
	"strings"
)

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
