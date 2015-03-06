package main

import (
	"testing"
)

// TestNewerVersion checks that in all cases, newerVesion returns the correct
// result.
func TestNewerVersion(t *testing.T) {
	// If the VERSION is changed, these tests might no longer be valid.
	if VERSION != "0.2.0" {
		t.Fatal("Need to update version tests")
	}

	if newerVersion(VERSION) {
		t.Error("Comparing to the current version should return false.")
	}
	if newerVersion("0.1") {
		t.Error("Comparing to 0.1 should return false")
	}
	if newerVersion("0.1.1") {
		t.Error("Comparing to 0.1.1 should return false")
	}
	if !newerVersion("1") {
		t.Error("Comparing to 1 should return true")
	}
	if !newerVersion("0.3") {
		t.Error("Comparing to 0.3 should return true")
	}
	if !newerVersion("0.2.1") {
		t.Error("Comparing to 0.2.1 should return true")
	}
	if !newerVersion("0.2.0.0") {
		t.Error("Comparing to 0.2.0.0 should return true")
	}
	if !newerVersion("0.2.0.1") {
		t.Error("Comparing to 0.2.0.1 should return true")
	}
}
