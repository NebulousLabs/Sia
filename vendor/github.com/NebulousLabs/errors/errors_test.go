package errors

import (
	"errors"
	"os"
	"testing"
)

var (
	errOne   = errors.New("one")
	errTwo   = errors.New("two")
	errThree = errors.New("three")
	errFour  = errors.New("four")
)

// TestError checks the Error method of the Error.
func TestError(t *testing.T) {
	r := Error{ErrSet: []error{errOne, errTwo}}
	if r.Error() != "[one; two]" {
		t.Error("got the wrong error output:", r.Error())
	}

	err := func() error {
		return r
	}()
	if err.Error() != r.Error() {
		t.Error("rich error not transcribing well")
	}
}

// TestComposeAndExtendErrs checks that two errors get composed and extended
// correctly.
func TestComposeAndExtendErrs(t *testing.T) {
	// Try composing one and two.
	err := Compose(errOne, errTwo)
	if err.Error() != "[one; two]" {
		t.Error("errors are being composed incorrectly:", err)
	}
	if !Contains(err, errOne) {
		t.Error("errors components not being checked correctly")
	}
	if !Contains(err, errTwo) {
		t.Error("errors componenets are not being checked correctly")
	}
	if Contains(errTwo, err) {
		// The cmp contains the base, which is not what the function is
		// checking.
		t.Error("errors components not being checked correctly")
	}

	// Check when errOne is nil.
	err = Compose(nil, errTwo)
	if err.Error() != "[two]" {
		t.Error("errors are being composed incorrectly:", err)
	}
	if Contains(err, errOne) {
		t.Error("errors components not being checked correctly")
	}
	if !Contains(err, errTwo) {
		t.Error("errors componenets are not being checked correctly")
	}

	// Check when errTwo is nil.
	err = Compose(errOne, nil)
	if err.Error() != "[one]" {
		t.Error("errors are being composed incorrectly:", err)
	}
	if !Contains(err, errOne) {
		t.Error("errors components not being checked correctly")
	}
	if Contains(err, errTwo) {
		t.Error("errors componenets are not being checked correctly")
	}

	// Check when both are nil.
	err = Compose(nil, nil)
	if err != nil {
		t.Error("nils are being composed incorrectly")
	}

	// Check the composition of Errors.
	err1 := Compose(errOne)
	err2 := Compose(errTwo, errThree)
	err3 := Compose(err1, err2)
	if err3.Error() != "[[one]; [two; three]]" {
		t.Error("error composition is happening incorrectly", err3.Error())
	}
	if !Contains(err3, errOne) {
		t.Error("rich errors are not being composed correctly")
	}
	if !Contains(err3, errTwo) {
		t.Error("rich errors are not being composed correctly")
	}
	if !Contains(err3, errThree) {
		t.Error("rich errors are not being composed correctly")
	}
	if Contains(err2, errOne) {
		t.Error("objects are getting confused")
	}

	// Extend err3, which should result in a three part error instead of adding
	// a new nesting.
	err3 = Extend(err3, errFour)
	if err3.Error() != "[four; [one]; [two; three]]" {
		t.Error("error extensions not working correctly", err3)
	}
	if IsOSNotExist(err3) {
		t.Error("error improperly recognized as os.IsNotExist")
	}

	// Extend a core error multiple times, make sure the outcome is desired.
	fullErr := Extend(Extend(Extend(errOne, errTwo), errThree), errFour)
	if fullErr.Error() != "[four; three; two; one]" {
		t.Error("iterated extensions are not working correctly", fullErr.Error())
	}
	if IsOSNotExist(fullErr) {
		t.Error("error improperly recognized as os.IsNotExist")
	}
}

// TestIsOSNotExist checks that not exist errors are validating correctly even
// when nested deeply.
func TestIsOSNotExist(t *testing.T) {
	_, err := os.Open("48718716928376918726591872.txt")
	if !IsOSNotExist(err) {
		t.Error("baseline os not exist error not recognized")
	}

	err = Extend(err, errOne)
	err = Extend(err, errTwo)
	err1 := Compose(errTwo, errThree, errFour)
	err = Compose(errOne, err, err1)
	err = Compose(nil, nil, nil, err)
	err = Compose(err, nil, nil)
	if !IsOSNotExist(err) {
		t.Error("After being buried, error is not  recognized as os.IsNotExist")
	}
}
