package api

import (
	"errors"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/inconshreveable/go-update"
	"github.com/kardianos/osext"
)

type UpdateInfo struct {
	Available bool
	Version   string
}

const (
	VERSION = "0.3.1"

	developerKey = `TODO: GENERATE DEVELOPER KEY`
)

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

// getHTTP is a helper function that returns the full response of an HTTP call
// to the update server.
func getHTTP(version, filename string) ([]byte, error) {
	resp, err := http.Get(updateURL + "/" + version + "/" + filename)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// fetchManifest requests and parses the update manifest. It returns the
// manifest (if available) as a slice of lines.
func fetchManifest(version string) (lines []string, err error) {
	manifest, err := getHTTP(version, "MANIFEST")
	if err != nil {
		return
	}
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

	// Get the executable directory.
	binDir, err := osext.ExecutableFolder()
	if err != nil {
		return
	}

	// Initialize the updater object.
	up, err := update.New().VerifySignatureWithPEM([]byte(developerKey))
	if err != nil {
		// should never happen
		return
	}

	// Perform updates as indicated by the manifest.
	for _, file := range manifest[1:] {
		// fetch the signature
		var sig []byte
		sig, err = getHTTP(version, file+".sig")
		if err != nil {
			return
		}
		// perform the update
		target := filepath.Join(binDir, file)
		err, _ = up.Target(target).VerifySignature(sig).FromUrl(updateURL + "/" + version + "/" + file)
		if err != nil {
			return
		}
	}

	return
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (srv *Server) daemonStopHandler(w http.ResponseWriter, req *http.Request) {
	// safely close each module
	srv.cs.Close()
	srv.gateway.Close()
	srv.wallet.Close()

	// can't write after we stop the server, so lie a bit.
	writeSuccess(w)

	// send stop signal
	srv.apiServer.Stop(time.Second)
}

// daemonUpdatesCheckHandler handles the API call to check for daemon updates.
func (srv *Server) daemonUpdatesCheckHandler(w http.ResponseWriter, req *http.Request) {
	available, version, err := checkForUpdate()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, UpdateInfo{available, version})
}

// daemonUpdatesApplyHandler handles the API call to apply daemon updates.
func (srv *Server) daemonUpdatesApplyHandler(w http.ResponseWriter, req *http.Request) {
	err := applyUpdate(req.FormValue("version"))
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeSuccess(w)
}
