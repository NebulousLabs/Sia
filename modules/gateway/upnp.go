package gateway

import (
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/NebulousLabs/go-upnp"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// myExternalIP discovers the gateway's external IP by querying a centralized
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
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	// trim newline
	return string(buf[:n-1]), nil
}

// threadedLearnHostname discovers the external IP of the Gateway. Once the IP
// has been discovered, it registers the ShareNodes RPC to be called on new
// connections, advertising the IP to other nodes.
func (g *Gateway) threadedLearnHostname() {
	if err := g.threads.Add(); err != nil {
		return
	}
	defer g.threads.Done()

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
		g.log.Println("WARN: failed to discover external IP:", err)
		return
	}

	g.mu.RLock()
	addr := modules.NetAddress(net.JoinHostPort(host, g.port))
	g.mu.RUnlock()
	if err := addr.IsValid(); err != nil {
		g.log.Printf("WARN: discovered hostname %q is invalid: %v", addr, err)
		return
	}

	g.mu.Lock()
	g.myAddr = addr
	g.mu.Unlock()

	g.log.Println("INFO: our address is", addr)
}

// threadedForwardPort adds a port mapping to the router.
func (g *Gateway) threadedForwardPort(port string) {
	if err := g.threads.Add(); err != nil {
		return
	}
	defer g.threads.Done()

	if build.Release == "testing" {
		return
	}

	d, err := upnp.Discover()
	if err != nil {
		g.log.Printf("WARN: could not automatically forward port %s: no UPnP-enabled devices found", port)
		return
	}

	portInt, _ := strconv.Atoi(port)
	err = d.Forward(uint16(portInt), "Sia RPC")
	if err != nil {
		g.log.Printf("WARN: could not automatically forward port %s: %v", port, err)
		return
	}

	g.log.Println("INFO: successfully forwarded port", port)
}

// clearPort removes a port mapping from the router.
func (g *Gateway) clearPort(port string) {
	if build.Release == "testing" {
		return
	}

	//d, err := upnp.Load("http://192.168.1.1:5000/Public_UPNP_gatedesc.xml")
	d, err := upnp.Discover()
	if err != nil {
		return
	}

	portInt, _ := strconv.Atoi(port)
	err = d.Clear(uint16(portInt))
	if err != nil {
		g.log.Printf("WARN: could not automatically unforward port %s: %v", port, err)
		return
	}

	g.log.Println("INFO: successfully unforwarded port", port)
}
