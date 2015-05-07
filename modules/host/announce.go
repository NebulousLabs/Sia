package host

import (
	"io"
	"net"
	"net/http"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// getExternalIP learns the host's hostname from a centralized service,
// myexternalip.com.
func (h *Host) getExternalIP() {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		// log?
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		// log?
		return
	}
	hostname := string(buf[:n-1]) // trim newline

	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)
	h.myAddr = modules.NetAddress(net.JoinHostPort(hostname, h.myAddr.Port()))
}

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() error {
	// create the transaction that will hold the announcement
	var t types.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return err
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	lockID := h.mu.RLock()
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: h.myAddr,
	})
	h.mu.RUnlock(lockID)
	_, _, err = h.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		return err
	}
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return err
	}

	// Add the transaction to the transaction pool.
	err = h.tpool.AcceptTransaction(t)
	if err != nil {
		return err
	}

	return nil
}
