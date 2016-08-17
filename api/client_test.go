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

// TestAuthenticatedApiClient tests that the API client connects to an
// authenticated server tester and can call and decode routes correctly, using
// the correct password.
func TestAuthenticatedApiClient(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testpass := "testPassword"
	st, err := createAuthenticatedServerTester("TestAuthenticatedApiClient", testpass)
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	c := NewClient("localhost:9980", "")
	var gatewayInfo GatewayGET
	err = c.Get("/gateway", &gatewayInfo)
	if err == nil {
		t.Fatal("api.Client did not return an error when requesting an authenticated resource without a password")
	}
	c = NewClient("localhost:9980", testpass)
	err = c.Get("/gateway", &gatewayInfo)
	if err != nil {
		t.Fatal(err)
	}
}
