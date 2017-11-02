package api

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCleanCloseHandler checks that if the handler keeps writing
// after cleanCloseHandler cancels it, no race condition happens.
// See https://github.com/NebulousLabs/Sia/issues/2385.
func TestCleanCloseHandler(t *testing.T) {
	t.Parallel()
	f := func(w http.ResponseWriter, r *http.Request) {
		buffer := make([]byte, 1000)
		for i := 0; i < 1e6; i++ {
			time.Sleep(time.Second / 1e6)
			w.Write(buffer)
		}
	}
	handler := cleanCloseHandler(http.HandlerFunc(f))
	server := httptest.NewServer(handler)
	url := "http://" + server.Listener.Addr().String()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	ctx := context.TODO()
	ctx, cancel := context.WithTimeout(ctx, time.Second/10)
	defer cancel()
	req = req.WithContext(ctx)
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer res.Body.Close()
	if _, err := ioutil.ReadAll(res.Body); err == nil {
		t.Fatalf("Expected to get timeout error")
	}
}
