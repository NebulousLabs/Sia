package host

import (
	"io"
	"net"
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// getExternalIP learns the server's hostname from a centralized service,
// myexternalip.com.
func getExternalIP() (string, error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	hostname := string(buf[:n-1]) // trim newline
	return hostname, nil
}

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() (err error) {
	// discover our external IP
	host, err := getExternalIP()
	if err != nil {
		return
	}
	_, port, err := net.SplitHostPort(h.listener.Addr().String())
	if err != nil {
		return
	}
	addr := modules.NetAddress(net.JoinHostPort(host, port))

	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)

	// create the transaction that will hold the announcement
	var t types.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: addr,
	})
	_, _, err = h.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		return
	}
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}

	// Add the transaction to the transaction pool.
	err = h.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
