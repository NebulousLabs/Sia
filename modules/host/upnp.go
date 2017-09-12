package host

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/NebulousLabs/go-upnp"
)

// managedLearnHostname discovers the external IP of the Host. If the host's
// net address is blank and the host's auto address appears to have changed,
// the host will make an announcement on the blockchain.
func (h *Host) managedLearnHostname() {
	if build.Release == "testing" {
		return
	}

	// Fetch a group of host vars that will be used to dictate the logic of the
	// function.
	h.mu.RLock()
	netAddr := h.settings.NetAddress
	hostPort := h.port
	hostAutoAddress := h.autoAddress
	hostAnnounced := h.announced
	hostAcceptingContracts := h.settings.AcceptingContracts
	hostContractCount := h.financialMetrics.ContractCount
	h.mu.RUnlock()

	// If the settings indicate that an address has been manually set, there is
	// no reason to learn the hostname.
	if netAddr != "" {
		return
	}
	h.log.Println("No manually set net address. Scanning to automatically determine address.")

	// try UPnP first, then fallback to myexternalip.com
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()
	var hostname string
	d, err := upnp.DiscoverCtx(ctx)
	if err == nil {
		hostname, err = d.ExternalIP()
	}
	if err != nil {
		hostname, err = myExternalIP()
	}
	if err != nil {
		h.log.Println("WARN: failed to discover external IP")
		return
	}

	autoAddress := modules.NetAddress(net.JoinHostPort(hostname, hostPort))
	if err := autoAddress.IsValid(); err != nil {
		h.log.Printf("WARN: discovered hostname %q is invalid: %v", autoAddress, err)
		return
	}
	if autoAddress == hostAutoAddress && hostAnnounced {
		// Nothing to do - the auto address has not changed and the previous
		// annoucement was successful.
		return
	}

	h.mu.Lock()
	h.autoAddress = autoAddress
	err = h.saveSync()
	h.mu.Unlock()
	if err != nil {
		h.log.Println(err)
	}

	// Announce the host, but only if the host is either accepting contracts or
	// has a storage obligation. If the host is not accepting contracts and has
	// no open contracts, there is no reason to notify anyone that the host's
	// address has changed.
	if hostAcceptingContracts || hostContractCount > 0 {
		h.log.Println("Host external IP address changed from", hostAutoAddress, "to", autoAddress, "- performing host announcement.")
		err = h.managedAnnounce(autoAddress)
		if err != nil {
			// Set h.announced to false, as the address has changed yet the
			// renewed annoucement has failed.
			h.mu.Lock()
			h.announced = false
			h.mu.Unlock()
			h.log.Println("unable to announce address after upnp-detected address change:", err)
		}
	}
}

// managedForwardPort adds a port mapping to the router.
func (h *Host) managedForwardPort(port string) error {
	if build.Release == "testing" {
		// Add a blocking placeholder where testing is able to mock behaviors
		// such as a port forward action that blocks for 10 seconds before
		// completing.
		if h.dependencies.Disrupt("managedForwardPort") {
			return nil
		}

		// Port forwarding functions are frequently unavailable during testing,
		// and the long blocking can be highly disruptive. Under normal
		// scenarios, return without complaint, and without running the
		// port-forward logic.
		return nil
	}

	// If the port is invalid, there is no need to perform any of the other
	// tasks.
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()
	d, err := upnp.DiscoverCtx(ctx)
	if err != nil {
		h.log.Printf("WARN: could not automatically forward port %s: %v", port, err)
		return err
	}
	err = d.Forward(uint16(portInt), "Sia Host")
	if err != nil {
		h.log.Printf("WARN: could not automatically forward port %s: %v", port, err)
		return err
	}

	h.log.Println("INFO: successfully forwarded port", port)
	return nil
}

// managedClearPort removes a port mapping from the router.
func (h *Host) managedClearPort() error {
	if build.Release == "testing" {
		// Allow testing to force an error to be returned here.
		if h.dependencies.Disrupt("managedClearPort return error") {
			return errors.New("Mocked managedClearPortErr")
		}
		return nil
	}

	// If the port is invalid, there is no need to perform any of the other
	// tasks.
	h.mu.RLock()
	port := h.port
	h.mu.RUnlock()
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-h.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()
	d, err := upnp.DiscoverCtx(ctx)
	if err != nil {
		return err
	}
	err = d.Clear(uint16(portInt))
	if err != nil {
		return err
	}

	h.log.Println("INFO: successfully unforwarded port", port)
	return nil
}

// myExternalIP discovers the host's external IP by querying a centralized
// service, http://myexternalip.com.
func myExternalIP() (string, error) {
	// timeout after 10 seconds
	client := http.Client{Timeout: time.Duration(10 * time.Second)}
	resp, err := client.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errResp, _ := ioutil.ReadAll(resp.Body)
		return "", errors.New(string(errResp))
	}
	buf, err := ioutil.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	if len(buf) == 0 {
		return "", errors.New("myexternalip.com returned a 0 length IP address")
	}
	// trim newline
	return strings.TrimSpace(string(buf)), nil
}
