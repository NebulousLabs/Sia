package api

import (
	"testing"
)

// TestNewerVersion checks that in all cases, newerVesion returns the correct
// result.
func TestNewerVersion(t *testing.T) {
	// If the VERSION is changed, these tests might no longer be valid.
	if VERSION != "0.3.1" {
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
	if !newerVersion("0.9") {
		t.Error("Comparing to 0.3 should return true")
	}
	if !newerVersion("0.3.2") {
		t.Error("Comparing to 0.3.2 should return true")
	}
	if !newerVersion("0.3.1.0") {
		t.Error("Comparing to 0.3.0.0 should return true")
	}
	if !newerVersion("0.3.1.1") {
		t.Error("Comparing to 0.3.0.1 should return true")
	}
}
