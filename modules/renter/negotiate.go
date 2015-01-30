package renter

import (
	"errors"
	"io"
	"net"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	// TODO: ask wallet
	minerFee = 10
)

func (r *Renter) createContractTransaction(host modules.HostEntry, up modules.UploadParams) (t consensus.Transaction, contract consensus.FileContract, err error) {
	// get state height
	r.state.RLock()
	height := r.state.Height()
	r.state.RUnlock()

	// Fill out the contract according to the whims of the host.
	contract = consensus.FileContract{
		FileMerkleRoot:     up.MerkleRoot,
		FileSize:           up.FileSize,
		Start:              height + up.Delay,
		End:                height + up.Delay + up.Duration,
		ValidProofAddress:  host.CoinAddress,
		MissedProofAddress: consensus.CoinAddress{}, // The empty address is the burn address.
	}

	// Create the transaction.
	id, err := r.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	fund := host.Price*consensus.Currency(up.Duration+up.Delay)*consensus.Currency(up.FileSize) + minerFee
	err = r.wallet.FundTransaction(id, fund)
	if err != nil {
		return
	}
	err = r.wallet.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = r.wallet.AddFileContract(id, contract)
	if err != nil {
		return
	}
	t, err = r.wallet.SignTransaction(id, false)
	if err != nil {
		return
	}

	return
}

func negotiateContract(host modules.HostEntry, t consensus.Transaction, up modules.UploadParams) error {
	return host.IPAddress.Call("NegotiateContract", func(conn net.Conn) (err error) {
		// send contract
		if _, err = encoding.WriteObject(conn, t); err != nil {
			return
		}
		// read response
		var response string
		if err = encoding.ReadObject(conn, &response, 128); err != nil {
			return
		}
		if response != modules.AcceptContractResponse {
			return errors.New(response)
		}
		// host accepted, so transmit file data
		// (no prefix needed, since the host already knows the filesize
		_, err = io.CopyN(conn, up.Data, int64(up.FileSize))
		// reset seek position
		up.Data.Seek(0, 0)
		return
	})
}
