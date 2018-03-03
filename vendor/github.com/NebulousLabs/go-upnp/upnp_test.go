package upnp

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestConcurrentUPNP tests that several threads calling Discover() concurrently
// succeed.
func TestConcurrentUPNP(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// verify that a router exists
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := DiscoverCtx(ctx)
	if err != nil {
		t.Skip(err)
	}

	// now try to concurrently Discover() using 20 threads
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := DiscoverCtx(ctx)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestIGD(t *testing.T) {
	// connect to router
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d, err := DiscoverCtx(ctx)
	if err != nil {
		t.Skip(err)
	}

	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Your external IP is:", ip)

	// forward a port
	err = d.Forward(9001, "upnp test")
	if err != nil {
		t.Fatal(err)
	}

	// un-forward a port
	err = d.Clear(9001)
	if err != nil {
		t.Fatal(err)
	}

	// record router's location
	loc := d.Location()
	if err != nil {
		t.Fatal(err)
	}

	// connect to router directly
	d, err = Load(loc)
	if err != nil {
		t.Fatal(err)
	}
}
