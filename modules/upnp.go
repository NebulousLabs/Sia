package modules

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NebulousLabs/go-upnp"

	"github.com/NebulousLabs/Sia/build"
)

var ErrNoUPnP = errors.New("UPnP not available")

// IGD is an interface to the local Internet Gateway Device. If no such device
// is available (e.g. if the user's router does not support UPnP), a dummy IGD
// is returned instead.
var IGD = func() upnp.IGD {
	// always use the loopback address during testing
	if build.Release == "testing" {
		return &dummyIGD{"::1"}
	}
	d, err := upnp.Discover()
	if err != nil {
		d = &dummyIGD{}
		d.ExternalIP() // cache external IP
		return d
	}
	return d
}()

// dummyIGD implements the upnp.IGD interface. It calls an external service to
// discover the external IP. Port mapping operations always return an error.
type dummyIGD struct {
	externalIP string
}

// ExternalIP discovers the computer's external IP by querying a centralized
// service. The result is cached after the first successful query.
func (d *dummyIGD) ExternalIP() (string, error) {
	if d.externalIP != "" {
		return d.externalIP, nil
	}

	// timeout after 3 seconds
	client := http.Client{Timeout: time.Duration(3 * time.Second)}
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
	d.externalIP = string(buf[:n-1]) // trim newline
	return d.externalIP, nil
}

// These methods are not supported by the dummyIGD.
func (d *dummyIGD) Forward(uint16, string) error { return ErrNoUPnP }
func (d *dummyIGD) Clear(uint16) error           { return ErrNoUPnP }
func (d *dummyIGD) Location() string             { return "" }
