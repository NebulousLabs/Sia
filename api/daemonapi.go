package api

import (
	"errors"
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/inconshreveable/go-update"
)

const VERSION = "0.3.0"

// TODO: Updates need to be signed!
// TODO: Updating on Windows may not work correctly.
// TODO: Will this code properly handle the case where multiple versions in a
// row have been missed?

// Updates work like this: each version is stored in a folder on a Linode
// server operated by the developers. The most recent version is stored in
// current/. The folder contains the files changed by the update, as well as a
// MANIFEST file that contains the version number and a file listing. To check
// for an update, we first read the version number from current/MANIFEST. If
// the version is newer, we download and apply the files listed in the update
// manifest.
var updateURL = "http://23.239.14.98/releases/" + runtime.GOOS + "_" + runtime.GOARCH

// newerVersion returns true if version is "greater than" VERSION.
func newerVersion(version string) bool {
	remote := strings.Split(VERSION, ".")
	local := strings.Split(version, ".")
	for i := range remote {
		ri, _ := strconv.Atoi(remote[i])
		li, _ := strconv.Atoi(local[i])
		if ri != li {
			return ri < li
		}
		if len(local)-1 == i {
			return false
		}
	}
	return true
}

// fetchManifest requests and parses the update manifest. It returns the
// manifest (if available) as a slice of lines.
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
func checkForUpdate() (bool, string, error) {
	manifest, err := fetchManifest("current")
	if err != nil {
		return false, "", err
	}
	version := manifest[0]
	return newerVersion(version), version, nil
}

// applyUpdate downloads and applies an update.
func applyUpdate(version string) (err error) {
	manifest, err := fetchManifest(version)
	if err != nil {
		return
	}

	// Perform updates as indicated by the manifest.
	for _, file := range manifest[1:] {
		err, _ = update.New().Target(file).FromUrl(updateURL + "/" + version + "/" + file)
		if err != nil {
			return
		}
	}

	// the binary must always be updated, because if nothing else, the version
	// number has to be bumped.
	err, _ = update.New().FromUrl(updateURL + "/" + version + "/siad")
	if err != nil {
		return
	}

	return
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (srv *Server) daemonStopHandler(w http.ResponseWriter, req *http.Request) {
	// safely close each module
	srv.state.Close()
	srv.gateway.Close()
	srv.wallet.Close()

	// can't write after we stop the server, so lie a bit.
	writeSuccess(w)

	// send stop signal
	srv.apiServer.Stop(time.Second)
}

// daemonUpdateCheckHandler handles the API call to check for daemon updates.
func (srv *Server) daemonUpdateCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := checkForUpdate()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, struct {
		Available bool
		Version   string
	}{available, version})
}

// daemonUpdateApplyHandler handles the API call to apply daemon updates.
func (srv *Server) daemonUpdateApplyHandler(w http.ResponseWriter, req *http.Request) {
	err := applyUpdate(req.FormValue("version"))
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
