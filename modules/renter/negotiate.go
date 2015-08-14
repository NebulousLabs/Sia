package renter

import (
	//"errors"
	//"io"
	"net"

	//"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// negotiateContract creates a file contract for a host according to the
// requests of the host. There is an assumption that only hosts with acceptable
// terms will be put into the hostdb.
func (r *Renter) negotiateContract(conn net.Conn, host modules.HostSettings) (types.FileContractID, error) {
	return types.FileContractID{}, nil
}

// revise updates an existing file contract with a host to include a new
// piece, which is uploaded to the host. It returns the piece's offset in the
// file contract data.
func (r *Renter) revise(fcid types.FileContractID, piece []byte) (uint64, error) {

	var offset uint64
	var fc types.FileContract

	// update contract in renter
	lockID := r.mu.Lock()
	r.contracts[fcid] = fc
	r.mu.Unlock(lockID)

	return offset, nil
}
