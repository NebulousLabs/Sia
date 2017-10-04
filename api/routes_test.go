package api

import (
	"testing"
)

func TestBuildHttpRoutes(t *testing.T) {
	api := &API{}
	router := buildHttpRoutes(api, "", "")
	if router == nil {
		t.Fatal("Failed to build routes.")
	}
}
