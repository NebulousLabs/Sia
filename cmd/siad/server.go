package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/explorer"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/NebulousLabs/Sia/types"

	"github.com/inconshreveable/go-update"
	"github.com/julienschmidt/httprouter"
	"github.com/kardianos/osext"
)

var errEmptyUpdateResponse = errors.New("API call to https://api.github.com/repos/NebulousLabs/Sia/releases/latest is returning an empty response")

type (
	// Server creates and serves a HTTP server that offers communication with a
	// Sia API.
	Server struct {
		httpServer    *http.Server
		listener      net.Listener
		config        Config
		moduleClosers []moduleCloser
		api           http.Handler
		mu            sync.Mutex
	}

	// moduleCloser defines a struct that closes modules, defined by a name and
	// an underlying io.Closer.
	moduleCloser struct {
		name string
		io.Closer
	}

	// SiaConstants is a struct listing all of the constants in use.
	SiaConstants struct {
		BlockFrequency         types.BlockHeight `json:"blockfrequency"`
		BlockSizeLimit         uint64            `json:"blocksizelimit"`
		ExtremeFutureThreshold types.Timestamp   `json:"extremefuturethreshold"`
		FutureThreshold        types.Timestamp   `json:"futurethreshold"`
		GenesisTimestamp       types.Timestamp   `json:"genesistimestamp"`
		MaturityDelay          types.BlockHeight `json:"maturitydelay"`
		MedianTimestampWindow  uint64            `json:"mediantimestampwindow"`
		SiafundCount           types.Currency    `json:"siafundcount"`
		SiafundPortion         *big.Rat          `json:"siafundportion"`
		TargetWindow           types.BlockHeight `json:"targetwindow"`

		InitialCoinbase uint64 `json:"initialcoinbase"`
		MinimumCoinbase uint64 `json:"minimumcoinbase"`

		RootTarget types.Target `json:"roottarget"`
		RootDepth  types.Target `json:"rootdepth"`

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp   *big.Rat `json:"maxadjustmentup"`
		MaxAdjustmentDown *big.Rat `json:"maxadjustmentdown"`

		MaxTargetAdjustmentUp   *big.Rat `json:"maxtargetadjustmentup"`
		MaxTargetAdjustmentDown *big.Rat `json:"maxtargetadjustmentdown"`

		SiacoinPrecision types.Currency `json:"siacoinprecision"`
	}

	// DaemonVersion holds the version information for siad
	DaemonVersion struct {
		Version     string `json:"version"`
		GitRevision string `json:"gitrevision"`
		BuildTime   string `json:"buildtime"`
	}
	// UpdateInfo indicates whether an update is available, and to what
	// version.
	UpdateInfo struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}
	// githubRelease represents some of the JSON returned by the GitHub release API
	// endpoint. Only the fields relevant to updating are included.
	githubRelease struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name        string `json:"name"`
			DownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
)

const (
	// The developer key is used to sign updates and other important Sia-
	// related information.
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

// version returns the version number of a non-LTS release. This assumes that
// tag names will always be of the form "vX.Y.Z".
func (r *githubRelease) version() string {
	return strings.TrimPrefix(r.TagName, "v")
}

// byVersion sorts non-LTS releases by their version string, placing the highest
// version number first.
type byVersion []githubRelease

func (rs byVersion) Len() int      { return len(rs) }
func (rs byVersion) Swap(i, j int) { rs[i], rs[j] = rs[j], rs[i] }
func (rs byVersion) Less(i, j int) bool {
	// we want the higher version number to reported as "less" so that it is
	// placed first in the slice
	return build.VersionCmp(rs[i].version(), rs[j].version()) >= 0
}

// latestRelease returns the latest non-LTS release, given a set of arbitrary
// releases.
func latestRelease(releases []githubRelease) (githubRelease, error) {
	// filter the releases to exclude LTS releases
	nonLTS := releases[:0]
	for _, r := range releases {
		if !strings.Contains(r.TagName, "lts") && build.IsVersion(r.version()) {
			nonLTS = append(nonLTS, r)
		}
	}

	// sort by version
	sort.Sort(byVersion(nonLTS))

	// return the latest release
	if len(nonLTS) == 0 {
		return githubRelease{}, errEmptyUpdateResponse
	}
	return nonLTS[0], nil
}

// fetchLatestRelease returns metadata about the most recent non-LTS GitHub
// release.
func fetchLatestRelease() (githubRelease, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/NebulousLabs/Sia/releases", nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	var releases []githubRelease
	err = json.NewDecoder(resp.Body).Decode(&releases)
	if err != nil {
		return githubRelease{}, err
	}
	return latestRelease(releases)
}

// updateToRelease updates siad and siac to the release specified. siac is
// assumed to be in the same folder as siad.
func updateToRelease(release githubRelease) error {
	updateOpts := update.Options{
		Verifier: update.NewRSAVerifier(),
	}
	err := updateOpts.SetPublicKeyPEM([]byte(developerKey))
	if err != nil {
		// should never happen
		return err
	}

	binaryFolder, err := osext.ExecutableFolder()
	if err != nil {
		return err
	}

	// construct release filename
	releaseName := fmt.Sprintf("Sia-%s-%s-%s.zip", release.TagName, runtime.GOOS, runtime.GOARCH)

	// find release
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == releaseName {
			downloadURL = asset.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return errors.New("couldn't find download URL for " + releaseName)
	}

	// download release archive
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	// release should be small enough to store in memory (<10 MiB); use
	// LimitReader to ensure we don't download more than 32 MiB
	content, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1<<25))
	resp.Body.Close()
	if err != nil {
		return err
	}
	r := bytes.NewReader(content)
	z, err := zip.NewReader(r, r.Size())
	if err != nil {
		return err
	}

	// process zip, finding siad/siac binaries and signatures
	for _, binary := range []string{"siad", "siac"} {
		var binData io.ReadCloser
		var signature []byte
		var binaryName string // needed for TargetPath below
		for _, zf := range z.File {
			switch base := path.Base(zf.Name); base {
			case binary, binary + ".exe":
				binaryName = base
				binData, err = zf.Open()
				if err != nil {
					return err
				}
				defer binData.Close()
			case binary + ".sig", binary + ".exe.sig":
				sigFile, err := zf.Open()
				if err != nil {
					return err
				}
				defer sigFile.Close()
				signature, err = ioutil.ReadAll(sigFile)
				if err != nil {
					return err
				}
			}
		}
		if binData == nil {
			return errors.New("could not find " + binary + " binary")
		} else if signature == nil {
			return errors.New("could not find " + binary + " signature")
		}

		// apply update
		updateOpts.Signature = signature
		updateOpts.TargetMode = 0775 // executable
		updateOpts.TargetPath = filepath.Join(binaryFolder, binaryName)
		err = update.Apply(binData, updateOpts)
		if err != nil {
			return err
		}
	}

	return nil
}

// daemonUpdateHandlerGET handles the API call that checks for an update.
func (srv *Server) daemonUpdateHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		api.WriteError(w, api.Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	latestVersion := release.TagName[1:] // delete leading 'v'
	api.WriteJSON(w, UpdateInfo{
		Available: build.VersionCmp(latestVersion, build.Version) > 0,
		Version:   latestVersion,
	})
}

// daemonUpdateHandlerPOST handles the API call that updates siad and siac.
// There is no safeguard to prevent "updating" to the same release, so callers
// should always check the latest version via daemonUpdateHandlerGET first.
// TODO: add support for specifying version to update to.
func (srv *Server) daemonUpdateHandlerPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		api.WriteError(w, api.Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	err = updateToRelease(release)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			api.WriteError(w, api.Error{Message: "Serious error: Failed to rollback from bad update: " + rerr.Error()}, http.StatusInternalServerError)
		} else {
			api.WriteError(w, api.Error{Message: "Failed to apply update: " + err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	api.WriteSuccess(w)
}

// debugConstantsHandler prints a json file containing all of the constants.
func (srv *Server) daemonConstantsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	sc := SiaConstants{
		BlockFrequency:         types.BlockFrequency,
		BlockSizeLimit:         types.BlockSizeLimit,
		ExtremeFutureThreshold: types.ExtremeFutureThreshold,
		FutureThreshold:        types.FutureThreshold,
		GenesisTimestamp:       types.GenesisTimestamp,
		MaturityDelay:          types.MaturityDelay,
		MedianTimestampWindow:  types.MedianTimestampWindow,
		SiafundCount:           types.SiafundCount,
		SiafundPortion:         types.SiafundPortion,
		TargetWindow:           types.TargetWindow,

		InitialCoinbase: types.InitialCoinbase,
		MinimumCoinbase: types.MinimumCoinbase,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxAdjustmentDown: types.MaxTargetAdjustmentDown,

		MaxTargetAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxTargetAdjustmentDown: types.MaxTargetAdjustmentDown,

		SiacoinPrecision: types.SiacoinPrecision,
	}

	api.WriteJSON(w, sc)
}

// daemonVersionHandler handles the API call that requests the daemon's version.
func (srv *Server) daemonVersionHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteJSON(w, DaemonVersion{Version: build.Version, GitRevision: build.GitRevision, BuildTime: build.BuildTime})
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (srv *Server) daemonStopHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// can't write after we stop the server, so lie a bit.
	api.WriteSuccess(w)

	// need to flush the response before shutting down the server
	f, ok := w.(http.Flusher)
	if !ok {
		panic("Server does not support flushing")
	}
	f.Flush()

	if err := srv.Close(); err != nil {
		build.Critical(err)
	}
}

func (srv *Server) daemonHandler(password string) http.Handler {
	router := httprouter.New()

	router.GET("/daemon/constants", srv.daemonConstantsHandler)
	router.GET("/daemon/version", srv.daemonVersionHandler)
	router.GET("/daemon/update", srv.daemonUpdateHandlerGET)
	router.POST("/daemon/update", srv.daemonUpdateHandlerPOST)
	router.GET("/daemon/stop", api.RequirePassword(srv.daemonStopHandler, password))

	return router
}

// apiHandler handles all calls to the API. If the ready flag is not set, this
// will return an error. Otherwise it will serve the api.
func (srv *Server) apiHandler(w http.ResponseWriter, r *http.Request) {
	srv.mu.Lock()
	isReady := srv.api != nil
	srv.mu.Unlock()
	if !isReady {
		api.WriteError(w, api.Error{Message: "siad is not ready. please wait for siad to finish loading."}, http.StatusServiceUnavailable)
		return
	}
	srv.api.ServeHTTP(w, r)
}

// NewServer creates a new net.http server listening on bindAddr.  Only the
// /daemon/ routes are registered by this func, additional routes can be
// registered later by calling serv.mux.Handle.
func NewServer(config Config) (*Server, error) {
	// Process the config variables after they are parsed by cobra.
	config, err := processConfig(config)
	if err != nil {
		return nil, err
	}
	// Create the listener for the server
	l, err := net.Listen("tcp", config.Siad.APIaddr)
	if err != nil {
		if isAddrInUseErr(err) {
			return nil, fmt.Errorf("%v; are you running another instance of siad?", err.Error())
		}

		return nil, err
	}

	// Create the Server
	mux := http.NewServeMux()
	srv := &Server{
		listener: l,
		httpServer: &http.Server{
			Handler: mux,

			// set reasonable timeout windows for requests, to prevent the Sia API
			// server from leaking file descriptors due to slow, disappearing, or
			// unreliable API clients.

			// ReadTimeout defines the maximum amount of time allowed to fully read
			// the request body. This timeout is applied to every handler in the
			// server.
			ReadTimeout: time.Minute * 5,

			// ReadHeaderTimeout defines the amount of time allowed to fully read the
			// request headers.
			ReadHeaderTimeout: time.Minute * 2,

			// IdleTimeout defines the maximum duration a HTTP Keep-Alive connection
			// the API is kept open with no activity before closing.
			IdleTimeout: time.Minute * 5,
		},
		config: config,
	}

	// Register siad routes
	mux.Handle("/daemon/", api.RequireUserAgent(srv.daemonHandler(config.APIPassword), config.Siad.RequiredUserAgent))
	mux.HandleFunc("/", srv.apiHandler)

	return srv, nil
}

// isAddrInUseErr checks if the error corresponds to syscall.EADDRINUSE
func isAddrInUseErr(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*os.SyscallError); ok {
			return syscallErr.Err == syscall.EADDRINUSE
		}
	}
	return false
}

// loadModules loads the modules defined by the server's config and makes their
// API routes available.
func (srv *Server) loadModules() error {
	// Create the server and start serving daemon routes immediately.
	fmt.Printf("(0/%d) Loading siad...\n", len(srv.config.Siad.Modules))

	// Initialize the Sia modules
	i := 0
	var err error
	var g modules.Gateway
	if strings.Contains(srv.config.Siad.Modules, "g") {
		i++
		fmt.Printf("(%d/%d) Loading gateway...\n", i, len(srv.config.Siad.Modules))
		g, err = gateway.New(srv.config.Siad.RPCaddr, !srv.config.Siad.NoBootstrap, filepath.Join(srv.config.Siad.SiaDir, modules.GatewayDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "gateway", Closer: g})
	}
	var cs modules.ConsensusSet
	if strings.Contains(srv.config.Siad.Modules, "c") {
		i++
		fmt.Printf("(%d/%d) Loading consensus...\n", i, len(srv.config.Siad.Modules))
		cs, err = consensus.New(g, !srv.config.Siad.NoBootstrap, filepath.Join(srv.config.Siad.SiaDir, modules.ConsensusDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "consensus", Closer: cs})
	}
	var e modules.Explorer
	if strings.Contains(srv.config.Siad.Modules, "e") {
		i++
		fmt.Printf("(%d/%d) Loading explorer...\n", i, len(srv.config.Siad.Modules))
		e, err = explorer.New(cs, filepath.Join(srv.config.Siad.SiaDir, modules.ExplorerDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "explorer", Closer: e})
	}
	var tpool modules.TransactionPool
	if strings.Contains(srv.config.Siad.Modules, "t") {
		i++
		fmt.Printf("(%d/%d) Loading transaction pool...\n", i, len(srv.config.Siad.Modules))
		tpool, err = transactionpool.New(cs, g, filepath.Join(srv.config.Siad.SiaDir, modules.TransactionPoolDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "transaction pool", Closer: tpool})
	}
	var w modules.Wallet
	if strings.Contains(srv.config.Siad.Modules, "w") {
		i++
		fmt.Printf("(%d/%d) Loading wallet...\n", i, len(srv.config.Siad.Modules))
		w, err = wallet.New(cs, tpool, filepath.Join(srv.config.Siad.SiaDir, modules.WalletDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "wallet", Closer: w})
	}
	var m modules.Miner
	if strings.Contains(srv.config.Siad.Modules, "m") {
		i++
		fmt.Printf("(%d/%d) Loading miner...\n", i, len(srv.config.Siad.Modules))
		m, err = miner.New(cs, tpool, w, filepath.Join(srv.config.Siad.SiaDir, modules.MinerDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "miner", Closer: m})
	}
	var h modules.Host
	if strings.Contains(srv.config.Siad.Modules, "h") {
		i++
		fmt.Printf("(%d/%d) Loading host...\n", i, len(srv.config.Siad.Modules))
		h, err = host.New(cs, tpool, w, srv.config.Siad.HostAddr, filepath.Join(srv.config.Siad.SiaDir, modules.HostDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "host", Closer: h})
	}
	var r modules.Renter
	if strings.Contains(srv.config.Siad.Modules, "r") {
		i++
		fmt.Printf("(%d/%d) Loading renter...\n", i, len(srv.config.Siad.Modules))
		r, err = renter.New(g, cs, w, tpool, filepath.Join(srv.config.Siad.SiaDir, modules.RenterDir))
		if err != nil {
			return err
		}
		srv.moduleClosers = append(srv.moduleClosers, moduleCloser{name: "renter", Closer: r})
	}

	// Create the Sia API
	a := api.New(
		srv.config.Siad.RequiredUserAgent,
		srv.config.APIPassword,
		cs,
		e,
		g,
		h,
		m,
		r,
		tpool,
		w,
	)

	// connect the API to the server
	srv.mu.Lock()
	srv.api = a
	srv.mu.Unlock()

	// Attempt to auto-unlock the wallet using the SIA_WALLET_PASSWORD env variable
	if password := os.Getenv("SIA_WALLET_PASSWORD"); password != "" {
		fmt.Println("Sia Wallet Password found, attempting to auto-unlock wallet")
		if err := unlockWallet(w, password); err != nil {
			fmt.Println("Auto-unlock failed.")
		} else {
			fmt.Println("Auto-unlock successful.")
		}
	}

	return nil
}

// Serve starts the HTTP server
func (srv *Server) Serve() error {
	// The server will run until an error is encountered or the listener is
	// closed, via either the Close method or the signal handling above.
	// Closing the listener will result in the benign error handled below.
	err := srv.httpServer.Serve(srv.listener)
	if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}

// Close closes the Server's listener, causing the HTTP server to shut down.
func (srv *Server) Close() error {
	var errs []error
	// Close the listener, which will cause Server.Serve() to return.
	if err := srv.listener.Close(); err != nil {
		errs = append(errs, err)
	}
	// Close all of the modules in reverse order
	for i := len(srv.moduleClosers) - 1; i >= 0; i-- {
		m := srv.moduleClosers[i]
		fmt.Printf("Closing %v...\n", m.name)
		if err := m.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return build.JoinErrors(errs, "\n")
}
