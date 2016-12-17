package hostdb

import (
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestDecrementReliability tests the decrementReliability method.
func TestDecrementReliability(t *testing.T) {
	hdb := bareHostDB()

	// Decrementing a non-existent host should be a no-op, and should build.Critical.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("decrementReliability should build.Critical with nonexistent host")
			}
		}()
		hdb.decrementReliability("foo", types.NewCurrency64(0))
	}()

	// Add a host to allHosts and activeHosts. Decrementing it should remove it
	// from activeHosts.
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.Reliability = types.NewCurrency64(1)
	hdb.allHosts[h.NetAddress] = h
	hdb.activeHosts[h.NetAddress] = h
	hdb.decrementReliability(h.NetAddress, types.NewCurrency64(0))
	if len(hdb.ActiveHosts()) != 0 {
		t.Error("decrementing did not remove host from activeHosts")
	}

	// Decrement reliability to 0. This should remove the host from allHosts.
	hdb.decrementReliability(h.NetAddress, h.Reliability)
	if len(hdb.AllHosts()) != 0 {
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
	t.Skip("incompatible concurrency patterns")
	if testing.Short() {
		t.SkipNow()
	}
	hdb := bareHostDB()
	hdb.persist = &memPersist{}

	// create a host to send to threadedProbeHosts
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.AcceptingContracts = true
	h.PublicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	h.Reliability = baseWeight // enough to withstand a few failures

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
	if len(hdb.ActiveHosts()) != 0 {
		t.Error("unresponsive host was added")
	}

	// make the RPC fail
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		ourPipe, theirPipe := net.Pipe()
		ourPipe.Close()
		return theirPipe, nil
	})
	runProbe(h)
	if len(hdb.ActiveHosts()) != 0 {
		t.Error("unresponsive host was added")
	}

	// normal host
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read the RPC
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			// write host settings
			crypto.WriteSignedObject(ourConn, modules.HostExternalSettings{
				AcceptingContracts: true,
				NetAddress:         "probed",
			}, sk)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	runProbe(h)
	if len(hdb.ActiveHosts()) != 1 {
		t.Error("host was not added")
	}
}

// TestThreadedProbeHostsCorruption tests the threadedProbeHosts method,
// specifically checking for corruption of the hostdb if the weight of a host
// changes after a scan.
func TestThreadedProbeHostsCorruption(t *testing.T) {
	t.Skip("incompatible concurrency patterns")
	if testing.Short() {
		t.SkipNow()
	}

	hdb := bareHostDB()
	hdb.persist = &memPersist{}

	// create a host to send to threadedProbeHosts
	sk, pk, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	h := new(hostEntry)
	h.NetAddress = "foo"
	h.AcceptingContracts = true
	h.PublicKey = types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}
	h.Reliability = baseWeight // enough to withstand a few failures

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

	// Add a normal host.
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read the RPC
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			// write host settings
			crypto.WriteSignedObject(ourConn, modules.HostExternalSettings{
				AcceptingContracts: true,
				StoragePrice:       types.NewCurrency64(15e6),
				NetAddress:         "probed",
			}, sk)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	runProbe(h)
	if len(hdb.ActiveHosts()) != 1 {
		t.Error("host was not added")
	}

	// Add the host again, this time changing the storage price, which will
	// change the weight of the host, which at one point would cause a
	// corruption of the host tree.
	hdb.dialer = probeDialer(func(modules.NetAddress, time.Duration) (net.Conn, error) {
		// create an in-memory conn and spawn a goroutine to handle our half
		ourConn, theirConn := net.Pipe()
		go func() {
			// read the RPC
			encoding.ReadObject(ourConn, new(types.Specifier), types.SpecifierLen)
			// write host settings
			crypto.WriteSignedObject(ourConn, modules.HostExternalSettings{
				AcceptingContracts: true,
				StoragePrice:       types.NewCurrency64(15e3), // Lower than the previous, to cause a higher weight.
				NetAddress:         "probed",
			}, sk)
			ourConn.Close()
		}()
		return theirConn, nil
	})
	runProbe(h)
	if len(hdb.ActiveHosts()) != 1 {
		t.Error("host was not added")
	}

	/*
		TODO
		// Check that the host tree has not been corrupted.
		err = repeatCheck(hdb.hostTree)
		if err != nil {
			t.Error(err)
		}
		err = uniformTreeVerification(hdb, 1)
		if err != nil {
			t.Error(err)
		}
	*/
}

// TestThreadedScan tests the threadedScan method.
func TestThreadedScan(t *testing.T) {
	hdb := bareHostDB()
	hdb.persist = &memPersist{}

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
	h.Reliability = types.NewCurrency64(1)
	hdb.activeHosts[h.NetAddress] = h

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
