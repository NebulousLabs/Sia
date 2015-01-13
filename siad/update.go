package main

import (
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/inconshreveable/go-update"
)

const VERSION = "0.1.0"

var updateURL = "http://23.239.14.98/releases/" + runtime.GOOS + "_" + runtime.GOARCH + "/current/"

// returns true if version is "greater than" VERSION.
func newerVersion(version string) bool {
	// super naive; assumes same number of .s
	// TODO: make this more robust... if it's worth the effort.
	nums := strings.Split(version, ".")
	NUMS := strings.Split(VERSION, ".")
	for i := range nums {
		// inputs are trusted, so no need to check the error
		ni, _ := strconv.Atoi(nums[i])
		Ni, _ := strconv.Atoi(NUMS[i])
		if ni > Ni {
			return true
		}
	}
	return false
}

// helper function that requests and parses the update manifest. It returns a
// boolean indicating whether an update is available, and a list of urls
// pointing to files targeted by the update.
func fetchManifest() (bool, []string, error) {
	resp, err := http.Get(updateURL + "MANIFEST")
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	manifest, _ := ioutil.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(manifest)), "\n")
	return newerVersion(lines[0]), lines[1:], nil
}

// checkForUpdate checks a centralized server for a more recent version of
// Sia, and returns true if an update is available.
func (d *daemon) checkForUpdate() (bool, error) {
	avail, _, err := fetchManifest()
	return avail, err
}

// applyUpdate downloads and applies an update. If Sia is up to date,
// applyUpdate is a no-op.
//
// TODO: lots of room for improvement here.
//   - binary diffs
//   - signed updates
//   - zipped updates
//   - ability to apply a specific update
func (d *daemon) applyUpdate() (applied bool, err error) {
	avail, files, err := fetchManifest()
	if !avail || err != nil {
		return
	}

	for _, file := range files {
		err, _ = update.New().Target(file).FromUrl(updateURL + file)
		if err != nil {
			// TODO: revert prior successful updates?
			break
		}
	}
	return
}
