package gateway

import (
	"net"
	"time"

	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/errors"
)

// discoverPeerIP is the handler for the discoverPeer RPC. It returns the
// public ip of the caller back to the caller. This allows for peer-to-peer ip
// discovery without centralized services.
func (g *Gateway) discoverPeerIP(conn modules.PeerConn) error {
	conn.SetDeadline(time.Now().Add(connStdDeadline))
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return errors.AddContext(err, "failed to split host from port")
	}
	return encoding.WriteObject(conn, host)
}

// managedIPFromPeers asks the peers the node is connected to for the node's
// public ip address. If not enough peers are available we wait a bit and try
// again. If no cancelation channel is provided, managedIPFromPeers will time
// out after timeoutIPDiscovery time. Otherwise it will time out when cancel is
// closed. The method might return with a short delay of
// peerDiscoveryRetryInterval.
func (g *Gateway) managedIPFromPeers(cancel <-chan struct{}) (string, error) {
	// Choose default if cancel is nil.
	var timeout <-chan time.Time
	if cancel == nil {
		timer := time.NewTimer(timeoutIPDiscovery)
		defer timer.Stop()
		timeout = timer.C
	}
	for {
		// Check for shutdown signal or timeout.
		select {
		case <-g.peerTG.StopChan():
			return "", errors.New("interrupted by shutdown")
		case <-timeout:
			return "", errors.New("failed to discover ip in time")
		case <-cancel:
			return "", errors.New("failed to discover ip in time")
		default:
		}
		// Get peers
		peers := g.Peers()
		// Check if there are enough peers. Otherwise wait.
		if len(peers) < minPeersForIPDiscovery {
			g.managedSleep(peerDiscoveryRetryInterval)
			continue
		}
		// Ask all the peers about our ip in parallel
		returnChan := make(chan string)
		for _, peer := range peers {
			go g.RPC(peer.NetAddress, "DiscoverIP", func(conn modules.PeerConn) error {
				var address string
				err := encoding.ReadObject(conn, &address, 100)
				if err != nil {
					returnChan <- ""
					g.log.Debugf("DEBUG: failed to receive ip address: %v", err)
					return err
				}
				addr := net.ParseIP(address)
				if addr == nil {
					returnChan <- ""
					g.log.Debug("DEBUG: failed to parse ip address")
					return errors.New("failed to parse ip address")
				}
				returnChan <- addr.String()
				return err
			})
		}
		// Wait for their responses
		addresses := make(map[string]int)
		successfulResponses := 0
		for i := 0; i < len(peers); i++ {
			addr := <-returnChan
			if addr != "" {
				addresses[addr]++
				successfulResponses++
			}
		}
		// If there haven't been enough successful responses we wait some time.
		if successfulResponses < minPeersForIPDiscovery {
			g.managedSleep(peerDiscoveryRetryInterval)
			continue
		}
		// If an address was returned by more than half the peers we consider
		// it valid.
		for addr, count := range addresses {
			if count > successfulResponses/2 {
				g.log.Println("ip successfully discovered using peers:", addr)
				return addr, nil
			}
		}
		// Otherwise we wait before trying again.
		g.managedSleep(peerDiscoveryRetryInterval)
	}
}
