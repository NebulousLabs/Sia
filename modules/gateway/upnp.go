package gateway

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NebulousLabs/errors"
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

// managedLearnHostname tries to discover the external ip of the machine. If
// discovering the address failed or if it is invalid, an error is returned.
func (g *Gateway) managedLearnHostname(cancel <-chan struct{}) (modules.NetAddress, error) {
	// create ctx to cancel upnp discovery during shutdown
	ctx, ctxCancel := context.WithTimeout(context.Background(), timeoutIPDiscovery)
	defer ctxCancel()
	go func() {
		select {
		case <-cancel:
			ctxCancel()
		case <-g.threads.StopChan():
			ctxCancel()
		case <-ctx.Done():
		}
	}()

	// try UPnP first, then fallback to myexternalip.com and peer-to-peer
	// discovery.
	var host string
	d, err := upnp.DiscoverCtx(ctx)
	if err == nil {
		host, err = d.ExternalIP()
	}
	if !build.DEBUG && err != nil {
		host, err = myExternalIP()
	}
	if err != nil {
		host, err = g.managedIPFromPeers(ctx.Done())
	}
	if err != nil {
		return "", errors.AddContext(err, "failed to discover external IP")
	}
	addr := modules.NetAddress(host)
	return addr, addr.IsValid()
}

// threadedLearnHostname discovers the external IP of the Gateway regularly.
func (g *Gateway) threadedLearnHostname() {
	if err := g.threads.Add(); err != nil {
		return
	}
	defer g.threads.Done()

	if build.Release == "testing" {
		return
	}

	for {
		host, err := g.managedLearnHostname(nil)
		if err != nil {
			g.log.Println("WARN: failed to discover external IP:", err)
		}
		// If we were unable to discover our IP we try again later.
		if err != nil {
			if !g.managedSleep(rediscoverIPIntervalFailure) {
				return // shutdown interrupted sleep
			}
			continue
		}

		g.mu.RLock()
		addr := modules.NetAddress(net.JoinHostPort(string(host), g.port))
		g.mu.RUnlock()
		if err := addr.IsValid(); err != nil {
			g.log.Printf("WARN: discovered hostname %q is invalid: %v", addr, err)
			if err != nil {
				if !g.managedSleep(rediscoverIPIntervalFailure) {
					return // shutdown interrupted sleep
				}
				continue
			}
		}

		g.mu.Lock()
		g.myAddr = addr
		g.mu.Unlock()

		g.log.Println("INFO: our address is", addr)

		// Rediscover the IP later in case it changed.
		if !g.managedSleep(rediscoverIPIntervalSuccess) {
			return // shutdown interrupted sleep
		}
	}
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-g.threads.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()
	d, err := upnp.DiscoverCtx(ctx)
	if err != nil {
		g.log.Printf("WARN: could not automatically forward port %s: no UPnP-enabled devices found: %v", port, err)
		return
	}

	portInt, _ := strconv.Atoi(port)
	err = d.Forward(uint16(portInt), "Sia RPC")
	if err != nil {
		g.log.Printf("WARN: could not automatically forward port %s: %v", port, err)
		return
	}

	g.log.Println("INFO: successfully forwarded port", port)

	// Establish port-clearing at shutdown.
	g.threads.AfterStop(func() {
		g.managedClearPort(port)
	})
}

// managedClearPort removes a port mapping from the router.
func (g *Gateway) managedClearPort(port string) {
	if build.Release == "testing" {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-g.threads.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()
	d, err := upnp.DiscoverCtx(ctx)
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
