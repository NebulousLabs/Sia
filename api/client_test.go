package api

import (
	"testing"
)

// TestApiClient tests that the API client connects to the server tester and
// can call and decode routes correctly.
func TestApiClient(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	st, err := createServerTester("TestApiClient")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	c := NewClient("localhost:9980", "")
	var gatewayInfo GatewayGET
	err = c.Get("/gateway", &gatewayInfo)
	if err != nil {
		t.Fatal(err)
	}
}
