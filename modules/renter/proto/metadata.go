package proto

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

// GetMetadata downloads sector ids from the host.
func GetMetadata(host modules.HostDBEntry, contract modules.RenterContract, cancel <-chan struct{}) ([]crypto.Hash, error) {
	conn, err := (&net.Dialer{
		Cancel:  cancel,
		Timeout: 15 * time.Second,
	}).Dial("tcp", string(host.NetAddress))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// allot 2 minutes for RPC request + revision exchange
	extendDeadline(conn, modules.NegotiateMetadataTime)
	if err := encoding.WriteObject(conn, modules.RPCMetadata); err != nil {
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	lastRevision, err := getRecentRevision(conn, contract, host.Version)
	if err != nil {
		return nil, err
	}
	numSectors := lastRevision.NewFileSize / modules.SectorSize
	begin, end := uint64(0), uint64(numSectors)
	if err := encoding.WriteObject(conn, begin); err != nil {
		return nil, errors.New("unable to write 'begin': " + err.Error())
	}
	if err := encoding.WriteObject(conn, end); err != nil {
		return nil, errors.New("unable to write 'end': " + err.Error())
	}
	// read acceptance
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return nil, errors.New("host did not accept [begin,end): " + err.Error())
	}
	var ids []crypto.Hash
	if err := encoding.ReadObject(conn, &ids, numSectors*crypto.HashSize+8); err != nil {
		return nil, errors.New("unable to read 'ids': " + err.Error())
	}
	// Calculate Merkle root from the ids, compare with the real root.
	log2SectorSize := uint64(0)
	for 1<<log2SectorSize < (modules.SectorSize / crypto.SegmentSize) {
		log2SectorSize++
	}
	tree := crypto.NewCachedTree(log2SectorSize)
	for _, sectorRoot := range ids {
		tree.Push(sectorRoot)
	}
	got := tree.Root()
	want := lastRevision.NewFileMerkleRoot
	if got != want {
		return nil, errors.New("sector ids do not match Merkle root")
	}
	return ids, nil
}
