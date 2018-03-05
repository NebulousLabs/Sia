package main

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node/api/client"
)

// TestLatestRelease tests that the latestRelease function properly processes a
// set of GitHub releases, returning the release with the highest version
// number.
func TestLatestRelease(t *testing.T) {
	tests := []struct {
		releases    []githubRelease
		expectedTag string
	}{
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v3.0.7"},
				{TagName: "lts-v2.0.0"},
			},
			expectedTag: "v3.0.7",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "v3.0.7"},
				{TagName: "v5.2.2"},
			},
			expectedTag: "v5.2.2",
		},
		{
			releases: []githubRelease{
				{TagName: "lts-v1.0.4"},
				{TagName: "lts-v1.0.7"},
				{TagName: "lts-v1.0.5"},
			},
			expectedTag: "", // no non-LTS versions
		},
		{
			releases: []githubRelease{
				{TagName: "v1.0.4"},
				{TagName: "v1.0.7"},
				{TagName: "v1.0.5"},
			},
			expectedTag: "v1.0.7",
		},
		{
			releases: []githubRelease{
				{TagName: "v1.0.4"},
				{TagName: "v1.0.4.1"},
				{TagName: "v1.0.4-patch1"},
			},
			expectedTag: "v1.0.4.1", // -patch is invalid
		},
		{
			releases: []githubRelease{
				{TagName: "abc"},
				{TagName: "def"},
				{TagName: "ghi"},
			},
			expectedTag: "", // invalid version strings
		},
	}
	for i, test := range tests {
		r, _ := latestRelease(test.releases)
		if r.TagName != test.expectedTag {
			t.Errorf("test %v failed: expected %q, got %q", i, test.expectedTag, r.TagName)
		}
	}
}

// TestNewServer verifies that NewServer creates a Sia API server correctly.
func TestNewServer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	var wg sync.WaitGroup
	config := Config{}
	config.Siad.APIaddr = "localhost:0"
	config.Siad.Modules = "cg"
	config.Siad.SiaDir = build.TempDir(t.Name())
	defer os.RemoveAll(config.Siad.SiaDir)
	srv, err := NewServer(config)
	if err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := srv.Serve()
		if err != nil {
			t.Fatal(err)
		}
	}()
	// verify that startup routes can be called correctly
	c := client.New(srv.listener.Addr().String())
	_, err = c.DaemonVersionGet()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ConsensusGet()
	if err == nil || !strings.Contains(err.Error(), "siad is not ready") {
		t.Fatal("expected consensus call on unloaded server to fail with siad not ready")
	}
	// create a goroutine that continuously makes API requests to test that
	// loading modules doesn't cause a race
	wg.Add(1)
	stopchan := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopchan:
				return
			default:
			}
			time.Sleep(time.Millisecond)
			c.ConsensusGet()
		}
	}()
	// load the modules, verify routes succeed
	err = srv.loadModules()
	if err != nil {
		t.Fatal(err)
	}
	close(stopchan)
	_, err = c.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	wg.Wait()
}
