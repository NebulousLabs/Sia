package proto

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// GetMetadata downloads sector IDs from the host.
func GetMetadata(host modules.HostDBEntry, fcid types.FileContractID, sk crypto.SecretKey, windowStart types.BlockHeight, begin, end uint64, cancel <-chan struct{}) (lastRevision types.FileContractRevision, ids []crypto.Hash, err error) {
	conn, err := (&net.Dialer{
		Cancel:  cancel,
		Timeout: 15 * time.Second,
	}).Dial("tcp", string(host.NetAddress))
	if err != nil {
		return
	}
	defer conn.Close()
	// allot 2 minutes for RPC request + revision exchange
	extendDeadline(conn, modules.NegotiateMetadataTime)
	if err = encoding.WriteObject(conn, modules.RPCMetadata); err != nil {
		err = errors.New("couldn't initiate RPC: " + err.Error())
		return
	}
	lastRevision, err = getRecentRevision(conn, fcid, sk, windowStart, host.Version)
	if err != nil {
		return
	}
	if err = encoding.WriteObject(conn, begin); err != nil {
		err = errors.New("unable to write 'begin': " + err.Error())
		return
	}
	if err = encoding.WriteObject(conn, end); err != nil {
		err = errors.New("unable to write 'end': " + err.Error())
		return
	}
	// read acceptance
	if err = modules.ReadNegotiationAcceptance(conn); err != nil {
		err = errors.New("host did not accept [begin,end): " + err.Error())
		return
	}
	numSectors := end - begin
	if err = encoding.ReadObject(conn, &ids, numSectors*crypto.HashSize+8); err != nil {
		err = errors.New("unable to read 'ids': " + err.Error())
		return
	}
	if uint64(len(ids)) != end-begin {
		err = errors.New("the host returned too short list of sector IDs")
		return
	}
	return
}
