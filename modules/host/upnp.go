package host

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/NebulousLabs/go-upnp"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

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
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	// trim newline
	return string(buf[:n-1]), nil
}

// managedLearnHostname discovers the external IP of the Host. If the host's
// net address is blank and the host's auto address appears to have changed,
// the host will make an announcement on the blockchain.
func (h *Host) managedLearnHostname() {
	if build.Release == "testing" {
		return
	}
	h.mu.RLock()
	netAddr := h.settings.NetAddress
	h.mu.RUnlock()
	// If the settings indicate that an address has been manually set, there is
	// no reason to learn the hostname.
	if netAddr != "" {
		return
	}

	// try UPnP first, then fallback to myexternalip.com
	var hostname string
	d, err := upnp.Discover()
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

	h.mu.Lock()
	defer h.mu.Unlock()
	autoAddress := modules.NetAddress(net.JoinHostPort(hostname, h.port))
	if autoAddress == h.autoAddress && h.announced {
		// Nothing to do - the auto address has not changed and the previous
		// annoucement was successful.
		return
	}
	err = h.announce(autoAddress)
	if err != nil {
		// Set h.announced to false, as the address has changed yet the
		// renewed annoucement has failed.
		h.announced = false
		h.log.Debugln(err)
	}
	h.autoAddress = autoAddress
	err = h.save()
	if err != nil {
		h.log.Println(err)
	}
}

// managedForwardPort adds a port mapping to the router.
func (h *Host) managedForwardPort() error {
	// If the port is invalid, there is no need to perform any of the other
	// tasks.
	h.mu.RLock()
	port := h.port
	h.mu.RUnlock()
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	if build.Release == "testing" {
		return nil
	}

	d, err := upnp.Discover()
	if err != nil {
		return err
	}
	err = d.Forward(uint16(portInt), "Sia Host")
	if err != nil {
		return err
	}

	h.log.Println("INFO: successfully forwarded port", port)
	return nil
}

// managedClearPort removes a port mapping from the router.
func (h *Host) managedClearPort() error {
	// If the port is invalid, there is no need to perform any of the other
	// tasks.
	h.mu.RLock()
	port := h.port
	h.mu.RUnlock()
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	if build.Release == "testing" {
		return nil
	}

	d, err := upnp.Discover()
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
