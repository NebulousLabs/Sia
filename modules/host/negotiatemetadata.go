package host

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// managedRPCMetadata accepts a request to get list of sector ids.
func (h *Host) managedRPCMetadata(conn net.Conn) error {
	// Perform the file contract revision exchange, giving the renter the most
	// recent file contract revision and getting the storage obligation that
	// will be used to get sector ids.
	_, so, err := h.managedRPCRecentRevision(conn)
	if err != nil {
		return extendErr("RPCRecentRevision failed: ", err)
	}
	// The storage obligation is received with a lock on it. Defer a call to
	// unlock the storage obligation.
	defer func() {
		h.managedUnlockStorageObligation(so.id())
	}()
	// Receive boundaries of so.SectorRoots to return.
	var begin, end uint64
	err = encoding.ReadObject(conn, &begin, 8)
	if err != nil {
		return extendErr("unable to read 'begin': ", ErrorConnection(err.Error()))
	}
	err = encoding.ReadObject(conn, &end, 8)
	if err != nil {
		return extendErr("unable to read 'end': ", ErrorConnection(err.Error()))
	}
	if end < begin {
		err = errors.New("Range error")
		modules.WriteNegotiationRejection(conn, err)
		return err
	}
	if end > uint64(len(so.SectorRoots)) {
		err = errors.New("Range out of bounds error")
		modules.WriteNegotiationRejection(conn, err)
		return err
	}
	if end-begin > modules.NegotiateMetadataMaxSliceSize {
		err = errors.New("The range is too long")
		modules.WriteNegotiationRejection(conn, err)
		return err
	}
	if err = modules.WriteNegotiationAcceptance(conn); err != nil {
		return extendErr("failed to write [begin,end) acceptance: ", ErrorConnection(err.Error()))
	}
	// Write roots of all sectors.
	err = encoding.WriteObject(conn, so.SectorRoots[begin:end])
	if err != nil {
		return extendErr("cound not write sectors: ", ErrorConnection(err.Error()))
	}
	return nil
}
