package main

import (
	"math/big"
	"testing"

	"gitlab.com/NebulousLabs/Sia/types"
)

func TestParseFilesize(t *testing.T) {
	tests := []struct {
		in, out string
		err     error
	}{
		{"1b", "1", nil},
		{"1KB", "1000", nil},
		{"1MB", "1000000", nil},
		{"1GB", "1000000000", nil},
		{"1TB", "1000000000000", nil},
		{"1KiB", "1024", nil},
		{"1MiB", "1048576", nil},
		{"1GiB", "1073741824", nil},
		{"1TiB", "1099511627776", nil},
		{"", "", errUnableToParseSize},
		{"123", "", errUnableToParseSize},
		{"123b", "123", nil},
		{"123TB", "123000000000000", nil},
		{"123GiB", "132070244352", nil},
		{"123BiB", "", errUnableToParseSize},
		{"GB", "", errUnableToParseSize},
		{"123G", "", errUnableToParseSize},
		{"123B99", "", errUnableToParseSize},
		{"12A3456", "", errUnableToParseSize},
		{"1.23KB", "1230", nil},
		{"1.234KB", "1234", nil},
		{"1.2345KB", "1234", nil},
	}
	for _, test := range tests {
		res, err := parseFilesize(test.in)
		if res != test.out || err != test.err {
			t.Errorf("parseFilesize(%v): expected %v %v, got %v %v", test.in, test.out, test.err, res, err)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		in, out string
		err     error
	}{
		{"x", "", errUnableToParseSize},
		{"1", "", errUnableToParseSize},
		{"b", "", errUnableToParseSize},
		{"1b", "1", nil},
		{"1 b", "1", nil},
		{"1block", "1", nil},
		{"1 block", "1", nil},
		{"1blocks", "1", nil},
		{"1 blocks", "1", nil},
		{"2b", "2", nil},
		{"2 b", "2", nil},
		{"2block", "2", nil},
		{"2 block", "2", nil},
		{"2blocks", "2", nil},
		{"2 blocks", "2", nil},
		{"2h", "12", nil},
		{"2 h", "12", nil},
		{"2hour", "12", nil},
		{"2 hour", "12", nil},
		{"2hours", "12", nil},
		{"2 hours", "12", nil},
		{"0.5d", "72", nil},
		{"0.5 d", "72", nil},
		{"0.5day", "72", nil},
		{"0.5 day", "72", nil},
		{"0.5days", "72", nil},
		{"0.5 days", "72", nil},
		{"10w", "10080", nil},
		{"10 w", "10080", nil},
		{"10week", "10080", nil},
		{"10 week", "10080", nil},
		{"10weeks", "10080", nil},
		{"10 weeks", "10080", nil},
		{"1 fortnight", "", errUnableToParseSize},
		{"three h", "", errUnableToParseSize},
	}
	for _, test := range tests {
		res, err := parsePeriod(test.in)
		if res != test.out || err != test.err {
			t.Errorf("parsePeriod(%v): expected %v %v, got %v %v", test.in, test.out, test.err, res, err)
		}
	}
}

func TestCurrencyUnits(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{"1", "1 H"},
		{"1000", "1000 H"},
		{"100000000000", "100000000000 H"},
		{"1000000000000", "1 pS"},
		{"1234560000000", "1.235 pS"},
		{"12345600000000", "12.35 pS"},
		{"123456000000000", "123.5 pS"},
		{"1000000000000000", "1 nS"},
		{"1000000000000000000", "1 uS"},
		{"1000000000000000000000", "1 mS"},
		{"1000000000000000000000000", "1 SC"},
		{"1000000000000000000000000000", "1 KS"},
		{"1000000000000000000000000000000", "1 MS"},
		{"1000000000000000000000000000000000", "1 GS"},
		{"1000000000000000000000000000000000000", "1 TS"},
		{"1234560000000000000000000000000000000", "1.235 TS"},
		{"1234560000000000000000000000000000000000", "1235 TS"},
	}
	for _, test := range tests {
		i, _ := new(big.Int).SetString(test.in, 10)
		out := currencyUnits(types.NewCurrency(i))
		if out != test.out {
			t.Errorf("currencyUnits(%v): expected %v, got %v", test.in, test.out, out)
		}
	}
}
