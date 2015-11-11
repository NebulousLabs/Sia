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
	h.myAddr = modules.NetAddress(net.JoinHostPort(host, h.myAddr.Port()))
	h.HostSettings.IPAddress = h.myAddr
	h.save()
	h.mu.Unlock()
}

// forwardPort adds a port mapping to the router.
func (h *Host) forwardPort(port string) {
	if build.Release == "testing" {
		return
	}

	d, err := upnp.Discover()
	if err != nil {
		h.log.Printf("WARN: could not automatically forward port %s: no UPnP-enabled devices found", port)
		return
	}

	portInt, _ := strconv.Atoi(port)
	err = d.Forward(uint16(portInt), "Sia Host")
	if err != nil {
		h.log.Printf("WARN: could not automatically forward port %s: %v", port, err)
		return
	}

	h.log.Println("INFO: successfully forwarded port", port)
}

// clearPort removes a port mapping from the router.
func (h *Host) clearPort(port string) {
	if build.Release == "testing" {
		return
	}

	d, err := upnp.Discover()
	if err != nil {
		return
	}

	portInt, _ := strconv.Atoi(port)
	err = d.Clear(uint16(portInt))
	if err != nil {
		h.log.Printf("WARN: could not automatically unforward port %s: %v", port, err)
		return
	}

	h.log.Println("INFO: successfully unforwarded port", port)
}
