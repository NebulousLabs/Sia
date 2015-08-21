package miningpool

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/ed25519"
)

type paymentChannel struct {
	// Total amount of money that can be sent through the payment channel
	size types.Currency

	// Address of the payment channel (the intermediate address)
	channelAddress types.UnlockHash

	// The address to pay to (e.g. the miner's address)
	payTo types.UnlockHash

	// The secret and public key for the address generated when the channel was
	// negotiated. These are needed to update the transaction when the miner
	// submits new work
	sk crypto.SecretKey
	pk crypto.PublicKey

	// The unlock conditions of the channel transactions. Contains the miner's
	// public key
	uc types.UnlockConditions

	// The channelOutputID (the ID of the txn funding the payment channel
	channelOutputID types.SiacoinOutputID

	// The transaction that refunds money to the pool (in case the miner
	// doesn't take their payout
	refundTxn types.Transaction
}

// Communicates with the miner to negotiate a payment channel for sending
// currency from the pool to the miner.
func (mp *MiningPool) rpcNegotiatePaymentChannel(conn net.Conn) error {
	// Get the miner's public key
	var minerPK crypto.PublicKey
	err := encoding.ReadObject(conn, &minerPK, ed25519.PublicKeySize)
	if err != nil {
		return err
	}

	// Create an address for this specific miner to send money to
	pc := paymentChannel{}
	pc.sk, pc.pk, err = crypto.GenerateSignatureKeys()
	if err != nil {
		return err
	}

	// Create a 2-of-2 unlock conditions. 1 key for the pool, 1 key for the
	// miner
	pc.uc = types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			{
				Algorithm: types.SignatureEd25519,
				Key:       pc.pk[:],
			},
			{
				Algorithm: types.SignatureEd25519,
				Key:       minerPK[:],
			},
		},
		SignaturesRequired: 2,
	}
	pc.channelAddress = pc.uc.UnlockHash()

	// Pool also creates, but does no sign a transaction that funds the channel
	// address.
	pc.size = types.SiacoinPrecision.Mul(types.NewCurrency64(300e3))
	fundingSK, fundingPK, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return err
	}
	fundingUC := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       fundingPK[:],
		}},
		SignaturesRequired: 1,
	}
	fundingAddr := fundingUC.UnlockHash()
	fundTxnBuilder := mp.wallet.StartTransaction()
	err = fundTxnBuilder.FundSiacoins(pc.size)
	if err != nil {
		return err
	}
	scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: pc.size, UnlockHash: fundingAddr})
	fundTxnSet, err := fundTxnBuilder.Sign(true)
	if err != nil {
		return err
	}
	fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
	channelTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         fundOutputID,
			UnlockConditions: fundingUC,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      pc.size,
			UnlockHash: pc.channelAddress,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(fundOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}

	// Send the unsigned channel-funding transaction to the miner
	err = encoding.WriteObject(conn, channelTxn)
	if err != nil {
		return err
	}

	// Pool creates and signs a transaction that refunds the channel to itself
	pc.channelOutputID = channelTxn.SiacoinOutputID(0)
	refundUC, err := mp.wallet.NextAddress()
	refundAddr := refundUC.UnlockHash()
	if err != nil {
		return err
	}
	pc.refundTxn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         pc.channelOutputID,
			UnlockConditions: pc.uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      pc.size,
			UnlockHash: refundAddr,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(pc.channelOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}
	sigHash := pc.refundTxn.SigHash(0)
	cryptoSig1, err := crypto.SignHash(sigHash, pc.sk)
	if err != nil {
		return err
	}
	pc.refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

	// Send the refund txn to the miner
	err = encoding.WriteObject(conn, pc.refundTxn)
	if err != nil {
		return err
	}

	// Receive the signed refundTxn from the miner
	err = encoding.ReadObject(conn, &pc.refundTxn, 10e3) // TODO: change to txn size
	if err != nil {
		return err
	}

	// TODO: Make sure it is signed
	// Pool now signs and broadcasts the funding transaction
	sigHash = channelTxn.SigHash(0)
	cryptoSig0, err := crypto.SignHash(sigHash, fundingSK)
	if err != nil {
		return err
	}
	channelTxn.TransactionSignatures[0].Signature = cryptoSig0[:]
	err = mp.tpool.AcceptTransactionSet(append(fundTxnSet, channelTxn))
	if err != nil {
		return err
	}

	// Pool receieves the miner's address so it knows the channel's output
	err = encoding.ReadObject(conn, &pc.payTo, 10e3) // TODO: Figure out how big the address is
	if err != nil {
		return err
	}

	// Remember this payment channel
	mp.channels[pc.payTo] = pc

	return nil
}

// sendPayment sends 'amount' through the payment channel whose recipient is
// `addr` by creating a new transaction that sends `amount + oldAmount` to
// `addr` and returning it. This new transaction is meant to then be sent to
// the miner off-chain
func (mp *MiningPool) sendPayment(amount types.Currency, addr types.UnlockHash) (types.Transaction, error) {
	pc, exists := mp.channels[addr]
	if !exists {
		return types.Transaction{}, errors.New("No payment channel exists with given output addr")
	}

	// TODO: Increment amount by how much the pool has already sent

	// TODO: Make sure amount is less than channel size
	pc.refundTxn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         pc.channelOutputID,
			UnlockConditions: pc.uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{
			{
				Value:      pc.size.Sub(amount),
				UnlockHash: mp.MiningPoolSettings.Address, // TODO: unique address per pc
			},
			{
				Value:      amount,
				UnlockHash: addr,
			},
		},
		TransactionSignatures: []types.TransactionSignature{
			{
				ParentID:       crypto.Hash(pc.channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
			{
				ParentID:       crypto.Hash(pc.channelOutputID),
				PublicKeyIndex: 1,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
		},
	}

	// Pool signs the output transaction
	sigHash := pc.refundTxn.SigHash(0)
	cryptoSig, err := crypto.SignHash(sigHash, pc.sk)
	if err != nil {
		return types.Transaction{}, err
	}
	pc.refundTxn.TransactionSignatures[0].Signature = cryptoSig[:]

	return pc.refundTxn, nil
}
