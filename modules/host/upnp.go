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

// learnHostname discovers the external IP of the Host. The Host cannot
// announce until the external IP is known.
func (h *Host) learnHostname() {
	if build.Release == "testing" {
		return
	}

	var host string

	// try UPnP first, then fallback to myexternalip.com
	d, err := upnp.Discover()
	if err == nil {
		host, err = d.ExternalIP()
	}
	if err != nil {
		host, err = myExternalIP()
	}
	if err != nil {
		h.log.Println("WARN: failed to discover external IP")
		return
	}

	h.mu.Lock()
	h.netAddress = modules.NetAddress(net.JoinHostPort(host, h.netAddress.Port()))
	h.settings.IPAddress = h.netAddress
	h.mu.Unlock()
}

// forwardPort adds a port mapping to the router.
func (h *Host) forwardPort(port string) error {
	// If the port is invalid, there is no need to perform any of the other
	// tasks.
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

// clearPort removes a port mapping from the router.
func (h *Host) clearPort(port string) error {
	// If the port is invalid, there is no need to perform any of the other
	// tasks.
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
