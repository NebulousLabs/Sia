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
	t.Parallel()
	st, err := createServerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	c := NewClient(st.server.listener.Addr().String(), "")
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
	t.Parallel()
	testpass := "testPassword"
	st, err := createAuthenticatedServerTester(t.Name(), testpass)
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.panicClose()

	c := NewClient(st.server.listener.Addr().String(), "")
	var walletAddress WalletAddressGET
	err = c.Get("/wallet/address", &walletAddress)
	if err == nil {
		t.Fatal("api.Client did not return an error when requesting an authenticated resource without a password")
	}
	c = NewClient(st.server.listener.Addr().String(), testpass)
	err = c.Get("/wallet/address", &walletAddress)
	if err != nil {
		t.Fatal(err)
	}
}
