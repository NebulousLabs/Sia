package api

import (
	"errors"
	"io/ioutil"
	"math/big"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/inconshreveable/go-update"
	"github.com/kardianos/osext"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

const (
	developerKey = `-----BEGIN PUBLIC KEY-----
MIIEIjANBgkqhkiG9w0BAQEFAAOCBA8AMIIECgKCBAEAsoQHOEU6s/EqMDtw5HvA
YPTUaBgnviMFbG3bMsRqSCD8ug4XJYh+Ik6WP0xgq+OPDehPiaXK8ghAtBiW1EJK
mBRwlABXAzREZg8wRfG4l8Zj6ckAPJOgLn0jobXy6/SCQ+jZSWh4Y8DYr+LA3Mn3
EOga7Jvhpc3fTZ232GBGJ1BobuNfRfYmwxSphv+T4vzIA3JUjVfa8pYZGIjh5XbJ
5M8Lef0Xa9eqr6lYm5kQoOIXeOW56ImqI2BKg/I9NGw9phSPbwaFfy1V2kfHp5Xy
DtKnyj/O9zDi+qUKjoIivnEoV+3DkioHUWv7Fpf7yx/9cPyckwvaBsTd9Cfp4uBx
qJ5Qyv69VZQiD6DikNwgzjGbIjiLwfTObhInKZUoYl48yzgkR80ja5TW0SoidNvO
4WTbWcLolOl522VarTs7wlgbq0Ad7yrNVnHzo447v2iT20ILH2oeAcZqvpcvRmTl
U6uKoaVmBH3D3Y19dPluOjK53BrqfQ5L8RFli2wEJktPsi5fUTd4UI9BgnUieuDz
S7h/VH9bv9ZVvyjpu/uVjdvaikT3zbIy9J6wS6uE5qPLPhI4B9HgbrQ03muDGpql
gZrMiL3GdYrBiqpIbaWHfM0eMWEK3ZScUdtCgUXMMrkvaUJ4g9wEgbONFVVOMIV+
YubIuzBFqug6WyxN/EAM/6Fss832AwVPcYM0NDTVGVdVplLMdN8YNjrYuaPngBCG
e8QaTWtHzLujyBIkVdAHqfkRS65jp7JLLMx7jUA74/E/v+0cNew3Y1p2gt3iQH8t
w93xn9IPUfQympc4h3KerP/Yn6P/qAh68jQkOiMMS+VbCq/BOn8Q3GbR+8rQ8dmk
qVoGA7XrPQ6bymKBTghk2Ek+ZjxrpAoj0xYoYyzWf0kuxeOT8kAjlLLmfQ8pm75S
QHLqH49FyfeETIU02rkw2oMOX/EYdJzZukHuouwbpKSElpRx+xTnaSemMJo+U7oX
xVjma3Zynh9w12abnFWkZKtrxwXv7FCSzb0UZmMWUqWzCS03Rrlur21jp4q2Wl71
Vt92xe5YbC/jbh386F1e/qGq6p+D1AmBynIpp/HE6fPsc9LWgJDDkREZcp7hthGW
IdYPeP3CesFHnsZMueZRib0i7lNUkBSRneO1y/C9poNv1vOeTCNEE0jvhp/XOJuc
yCQtrUSNALsvm7F+bnwP2F7K34k7MOlOgnTGqCqW+9WwBcjR44B0HI+YERCcRmJ8
krBuVo9OBMV0cYBWpjo3UI9j3lHESCYhLnCz7SPap7C1yORc2ydJh+qjKqdLBHom
t+JydcdJLbIG+kb3jB9QIIu5A4TlSGlHV6ewtxIWLS1473jEkITiVTt0Y5k+VLfW
bwIDAQAB
-----END PUBLIC KEY-----`
)

// Updates work like this: each version is stored in a folder on a Linode
// server operated by the developers. The most recent version is stored in
// current/. The folder contains the files changed by the update, as well as a
// MANIFEST file that contains the version number and a file listing. To check
// for an update, we first read the version number from current/MANIFEST. If
// the version is newer, we download and apply the files listed in the update
// manifest.
var updateURL = "http://23.239.14.98/releases/" + runtime.GOOS + "_" + runtime.GOARCH

// SiaConstants is a struct listing all of the constants in use.
type SiaConstants struct {
	BlockSizeLimit        uint64
	BlockFrequency        types.BlockHeight
	TargetWindow          types.BlockHeight
	MedianTimestampWindow uint64
	FutureThreshold       types.Timestamp
	SiafundCount          types.Currency
	SiafundPortion        *big.Rat

	InitialCoinbase uint64
	MinimumCoinbase uint64

	MaturityDelay types.BlockHeight

	GenesisTimestamp         types.Timestamp
	GenesisSiafundUnlockHash types.UnlockHash
	GenesisClaimUnlockHash   types.UnlockHash

	RootTarget types.Target
	RootDepth  types.Target

	MaxAdjustmentUp   *big.Rat
	MaxAdjustmentDown *big.Rat

	SiacoinPrecision types.Currency
}

type UpdateInfo struct {
	Available bool
	Version   string
}

// getHTTP is a helper function that returns the full response of an HTTP call
// to the update server.
func getHTTP(version, filename string) ([]byte, error) {
	resp, err := http.Get(updateURL + "/" + version + "/" + filename)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(string(data))
	}
	return data, err
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
	return build.VersionCmp(build.Version, version) < 0, version, nil
}

// applyUpdate downloads and applies an update.
func applyUpdate(version string) error {
	manifest, err := fetchManifest(version)
	if err != nil {
		return err
	}

	// Get the executable directory.
	binDir, err := osext.ExecutableFolder()
	if err != nil {
		return err
	}

	// Configure the update.
	var opts update.Options
	opts.SetPublicKeyPEM([]byte(developerKey))
	// Perform updates as indicated by the manifest.
	for _, file := range manifest[1:] {
		// set update path
		opts.TargetPath = filepath.Join(binDir, file)

		// fetch the signature
		opts.Signature, err = getHTTP(version, file+".sig")
		if err != nil {
			return err
		}

		// read update body
		var resp *http.Response
		resp, err = http.Get(updateURL + "/" + version + "/" + file)
		if err != nil {
			return err
		}
		err = update.Apply(resp.Body, opts)
		resp.Body.Close()
		if err != nil {
			return err
		}

	}

	return nil
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (srv *Server) daemonStopHandler(w http.ResponseWriter, req *http.Request) {
	// can't write after we stop the server, so lie a bit.
	writeSuccess(w)

	// send stop signal
	srv.apiServer.Stop(time.Second)
}

// daemonVersionHandler handles the API call that requests the daemon's version.
func (srv *Server) daemonVersionHandler(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, build.Version)
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

// debugConstantsHandler prints a json file containing all of the constants.
func (srv *Server) daemonConstantsHandler(w http.ResponseWriter, req *http.Request) {
	sc := SiaConstants{
		GenesisTimestamp:      types.GenesisTimestamp,
		BlockSizeLimit:        types.BlockSizeLimit,
		BlockFrequency:        types.BlockFrequency,
		TargetWindow:          types.TargetWindow,
		MedianTimestampWindow: types.MedianTimestampWindow,
		FutureThreshold:       types.FutureThreshold,
		SiafundCount:          types.SiafundCount,
		MaturityDelay:         types.MaturityDelay,
		SiafundPortion:        types.SiafundPortion,

		InitialCoinbase:  types.InitialCoinbase,
		MinimumCoinbase:  types.MinimumCoinbase,
		SiacoinPrecision: types.SiacoinPrecision,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		MaxAdjustmentUp:   types.MaxAdjustmentUp,
		MaxAdjustmentDown: types.MaxAdjustmentDown,
	}

	writeJSON(w, sc)
}
