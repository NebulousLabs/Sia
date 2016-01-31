package hostdb

import (
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestDecrementReliability tests the decrementReliability method.
func TestDecrementReliability(t *testing.T) {
	hdb := bareHostDB()

	// Decrementing a non-existent host should be a no-op.
	// NOTE: can't check any post-conditions here; only indication of correct
	// behavior is that the test doesn't panic.
	hdb.decrementReliability("foo", types.NewCurrency64(0))

	// Add a host to allHosts and activeHosts. Decrementing it should remove it
	// from activeHosts.
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.reliability = types.NewCurrency64(1)
	hdb.allHosts[h.NetAddress] = h
	hdb.activeHosts[h.NetAddress] = &hostNode{hostEntry: h}
	hdb.decrementReliability(h.NetAddress, types.NewCurrency64(0))
	if _, ok := hdb.activeHosts[h.NetAddress]; ok {
		t.Error("decrementing did not remove host from activeHosts")
	}

	// Decrement reliability to 0. This should remove the host from allHosts.
	hdb.decrementReliability(h.NetAddress, h.reliability)
	if _, ok := hdb.allHosts[h.NetAddress]; ok {
		t.Error("decrementing did not remove host from allHosts")
	}
}

// probeDialer is used to test the threadedProbeHosts method. A simple type
// alias is used so that it can easily be redefined during testing, allowing
// multiple behaviors to be tested.
type probeDialer func(modules.NetAddress, time.Duration) (net.Conn, error)

func (dial probeDialer) DialTimeout(addr modules.NetAddress, timeout time.Duration) (net.Conn, error) {
	return dial(addr, timeout)
}

// TestThreadedProbeHosts tests the threadedProbeHosts method.
func TestThreadedProbeHosts(t *testing.T) {
	hdb := bareHostDB()

	// create a host to send to threadedProbeHosts
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.reliability = baseWeight // enough to withstand a few failures

	// define a helper function for running threadedProbeHosts. We send the
	// hostEntry, close the channel, and then call threadedProbeHosts.
	// threadedProbeHosts will receive the host, loop once, and return after
	// seeing the channel has closed.
	runProbe := func(h *hostEntry) {
		hdb.scanPool <- h
		close(hdb.scanPool)
		hdb.threadedProbeHosts()
		// reset hdb.scanPool
		hdb.scanPool = make(chan *hostEntry, 1)
	}

	// make the dial fail
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		return nil, net.UnknownNetworkError("fail")
	})
	runProbe(h)
	if _, ok := hdb.activeHosts[h.NetAddress]; ok {
		t.Error("unresponsive host was added")
	}

	// make the RPC fail
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		ourPipe, theirPipe := net.Pipe()
		ourPipe.Close()
		return theirPipe, nil
	})
	runProbe(h)
	if _, ok := hdb.activeHosts[h.NetAddress]; ok {
		t.Error("unresponsive host was added")
	}

	// normal host
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read the RPC
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			// write old host settings
			encoding.WriteObject(ourConn, oldHostSettings{
				NetAddress: "probed",
			})
			ourConn.Close()
		}()
		return theirConn, nil
	})
	runProbe(h)
	if _, ok := hdb.activeHosts[h.NetAddress]; !ok {
		t.Error("host was not added")
	}

	// TODO: respond with old host settings
}

// TestThreadedScan tests the threadedScan method.
func TestThreadedScan(t *testing.T) {
	hdb := bareHostDB()
	// use a real sleeper; this will prevent threadedScan from looping too
	// quickly.
	hdb.sleeper = stdSleeper{}
	// use a dummy dialer that always fails
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		return nil, net.UnknownNetworkError("fail")
	})

	// create a host to be scanned
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.reliability = types.NewCurrency64(1)
	hdb.activeHosts[h.NetAddress] = &hostNode{hostEntry: h}

	// perform one scan
	go hdb.threadedScan()

	// host should be sent down scanPool
	select {
	case <-hdb.scanPool:
	case <-time.After(time.Second):
		t.Error("host was not scanned")
	}

	// remove the host from activeHosts and add it to allHosts
	hdb.mu.Lock()
	delete(hdb.activeHosts, h.NetAddress)
	hdb.allHosts[h.NetAddress] = h
	hdb.mu.Unlock()

	// perform one scan
	go hdb.threadedScan()

	// host should be sent down scanPool
	select {
	case <-hdb.scanPool:
	case <-time.After(time.Second):
		t.Error("host was not scanned")
	}
}
