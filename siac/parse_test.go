package main

import (
	"fmt"
	"testing"
)

func TestParseSize(t *testing.T) {
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
		{"123", "123", nil},
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
		res, err := parseSize(test.in)
		if res != test.out || err != test.err {
			t.Error(fmt.Sprintf("parseSize(%v): expected %v %v, got %v %v", test.in, test.out, test.err, res, err))
		}
	}
}
