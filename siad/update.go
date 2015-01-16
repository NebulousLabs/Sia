package main

import (
	"errors"
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
		if ni != Ni {
			return ni > Ni
		}
	}
	// versions are equal
	return false
}

// helper function that requests and parses the update manifest.
// It returns the manifest (if available) as a slice of lines.
func fetchManifest(version string) (lines []string, err error) {
	resp, err := http.Get(updateURL + "/" + version + "/MANIFEST")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	manifest, _ := ioutil.ReadAll(resp.Body)
	lines = strings.Split(strings.TrimSpace(string(manifest)), "\n")
	if len(lines) == 0 {
		err = errors.New("could not parse MANIFEST file")
	}
	return
}

// checkForUpdate checks a centralized server for a more recent version of
// Sia. If an update is available, it returns true, along with the newer
// version.
func (d *daemon) checkForUpdate() (bool, string, error) {
	manifest, err := fetchManifest("current")
	if err != nil {
		return false, "", err
	}
	version := manifest[0]
	return newerVersion(version), version, nil
}

// applyUpdate downloads and applies an update.
//
// TODO: lots of room for improvement here.
//   - binary diffs
//   - signed updates
//   - zipped updates
func (d *daemon) applyUpdate(version string) (err error) {
	manifest, err := fetchManifest(version)
	if err != nil {
		return
	}

	for _, file := range manifest[1:] {
		err, _ = update.New().Target(file).FromUrl(updateURL + "/" + version + "/" + file)
		if err != nil {
			// TODO: revert prior successful updates?
			return
		}
	}

	// the binary must always be updated, because if nothing else, the version
	// number has to be bumped.
	// TODO: should it be siad.exe on Windows?
	err, _ = update.New().FromUrl(updateURL + "/" + version + "/siad")
	if err != nil {
		return
	}

	return
}
