package main

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"gitlab.com/NebulousLabs/Sia/types"
)

var errUnableToParseSize = errors.New("unable to parse size")

// filesize returns a string that displays a filesize in human-readable units.
func filesizeUnits(size int64) string {
	if size == 0 {
		return "0 B"
	}
	sizes := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	i := int(math.Log10(float64(size)) / 3)
	return fmt.Sprintf("%.*f %s", i, float64(size)/math.Pow10(3*i), sizes[i])
}

// parseFilesize converts strings of form 10GB to a size in bytes. Fractional
// sizes are truncated at the byte size.
func parseFilesize(strSize string) (string, error) {
	units := []struct {
		suffix     string
		multiplier int64
	}{
		{"kb", 1e3},
		{"mb", 1e6},
		{"gb", 1e9},
		{"tb", 1e12},
		{"kib", 1 << 10},
		{"mib", 1 << 20},
		{"gib", 1 << 30},
		{"tib", 1 << 40},
		{"b", 1}, // must be after others else it'll match on them all
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

// periodUnits turns a period in terms of blocks to a number of weeks.
func periodUnits(blocks types.BlockHeight) string {
	return fmt.Sprint(blocks / 1008) // 1008 blocks per week
}

// parsePeriod converts a duration specified in blocks, hours, or weeks to a
// number of blocks.
func parsePeriod(period string) (string, error) {
	units := []struct {
		suffix     string
		multiplier float64
	}{
		{"b", 1},        // blocks
		{"block", 1},    // blocks
		{"blocks", 1},   // blocks
		{"h", 6},        // hours
		{"hour", 6},     // hours
		{"hours", 6},    // hours
		{"d", 144},      // days
		{"day", 144},    // days
		{"days", 144},   // days
		{"w", 1008},     // weeks
		{"week", 1008},  // weeks
		{"weeks", 1008}, // weeks
	}

	period = strings.ToLower(period)
	for _, unit := range units {
		if strings.HasSuffix(period, unit.suffix) {
			var base float64
			_, err := fmt.Sscan(strings.TrimSuffix(period, unit.suffix), &base)
			if err != nil {
				return "", errUnableToParseSize
			}
			blocks := int(base * unit.multiplier)
			return fmt.Sprint(blocks), nil
		}
	}

	return "", errUnableToParseSize
}

// currencyUnits converts a types.Currency to a string with human-readable
// units. The unit used will be the largest unit that results in a value
// greater than 1. The value is rounded to 4 significant digits.
func currencyUnits(c types.Currency) string {
	pico := types.SiacoinPrecision.Div64(1e12)
	if c.Cmp(pico) < 0 {
		return c.String() + " H"
	}

	// iterate until we find a unit greater than c
	mag := pico
	unit := ""
	for _, unit = range []string{"pS", "nS", "uS", "mS", "SC", "KS", "MS", "GS", "TS"} {
		if c.Cmp(mag.Mul64(1e3)) < 0 {
			break
		} else if unit != "TS" {
			// don't want to perform this multiply on the last iter; that
			// would give us 1.235 TS instead of 1235 TS
			mag = mag.Mul64(1e3)
		}
	}

	num := new(big.Rat).SetInt(c.Big())
	denom := new(big.Rat).SetInt(mag.Big())
	res, _ := new(big.Rat).Mul(num, denom.Inv(denom)).Float64()

	return fmt.Sprintf("%.4g %s", res, unit)
}

// parseCurrency converts a siacoin amount to base units.
func parseCurrency(amount string) (string, error) {
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

// yesNo returns "Yes" if b is true, and "No" if b is false.
func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
