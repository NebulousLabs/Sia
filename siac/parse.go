package main

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var errUnableToParseSize = errors.New("unable to parse size")

// parseSize converts strings of form 10GB to a size in bytes. Fractional sizes
// are truncated at the byte size. If parsing fails an error is returned.
func parseSize(strSize string) (string, error) {
	units := []struct {
		suffix     string
		multiplier int64
	}{
		{"kb", 1000},
		{"mb", 1000000},
		{"gb", 1000000000},
		{"tb", 1000000000000},
		{"kib", 1024},
		{"mib", 1048576},
		{"gib", 1073741824},
		{"tib", 1099511627776},
		{"b", 1}, // must be after others else it'll match on them all
		{"", 1},  // no suffix is still a valid suffix
	}

	strSize = strings.ToLower(strSize)
	for _, unit := range units {
		if strings.HasSuffix(strSize, unit.suffix) {
			r, ok := new(big.Rat).SetString(strings.TrimSuffix(strSize, unit.suffix))
			if !ok {
				return "", errUnableToParseSize
			}
			r.Mul(r, new(big.Rat).SetInt(big.NewInt(unit.multiplier)))
			if !r.IsInt() {
				f, _ := r.Float64()
				return fmt.Sprintf("%d", int64(f)), nil
			}
			return r.RatString(), nil
		}
	}

	return "", errUnableToParseSize
}

// coinUnits converts a siacoin amount to base units.
func coinUnits(amount string) (string, error) {
	units := []string{"pS", "nS", "uS", "mS", "SC", "KS", "MS", "GS", "TS"}
	for i, unit := range units {
		if strings.HasSuffix(amount, unit) {
			// scan into big.Rat
			r, ok := new(big.Rat).SetString(strings.TrimSuffix(amount, unit))
			if !ok {
				return "", errors.New("malformed amount")
			}
			// convert units
			exp := 24 + 3*(int64(i)-4)
			mag := new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
			r.Mul(r, new(big.Rat).SetInt(mag))
			// r must be an integer at this point
			if !r.IsInt() {
				return "", errors.New("non-integer number of hastings")
			}
			return r.RatString(), nil
		}
	}
	// check for hastings separately
	if strings.HasSuffix(amount, "H") {
		return strings.TrimSuffix(amount, "H"), nil
	}

	return "", errors.New("amount is missing units; run 'wallet --help' for a list of units")
}
