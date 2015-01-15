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

// Updates work like this: each version is stored in a folder on a Linode
// server operated by the developers. The most recent version is stored in
// current/. The folder contains the files changed by the update, as well as a
// MANIFEST file that contains the version number and a file listing. To check
// for an update, we first read the version number from current/MANIFEST. If
// the version is newer, we download and apply the files listed in the update
// manifest.
var updateURL = "http://23.239.14.98/releases/" + runtime.GOOS + "_" + runtime.GOARCH

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
func fetchManifest(version string) (string, []string, error) {
	resp, err := http.Get(updateURL + "/" + version + "/MANIFEST")
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	manifest, _ := ioutil.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(manifest)), "\n")
	return lines[0], lines[1:], nil
}

// checkForUpdate checks a centralized server for a more recent version of
// Sia, and returns true if an update is available.
func (d *daemon) checkForUpdate() (bool, error) {
	currentVersion, _, err := fetchManifest("current")
	return newerVersion(currentVersion), err
}

// applyUpdate downloads and applies an update.
//
// TODO: lots of room for improvement here.
//   - binary diffs
//   - signed updates
//   - zipped updates
func (d *daemon) applyUpdate(version string) (err error) {
	_, files, err := fetchManifest(version)
	if err != nil {
		return
	}

	for _, file := range files {
		err, _ = update.New().Target(file).FromUrl(updateURL + "/" + version + "/" + file)
		if err != nil {
			// TODO: revert prior successful updates?
			break
		}
	}
	return
}
