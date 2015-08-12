package miningpool

import (
	"fmt"
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

	// The transaction that refunds money to the pool (in case the miner
	// doesn't take their payout
	refundTxn types.Transaction
}

// Communicates with the miner to negotiate a payment channel for sending
// currency from the pool to the miner.
func (mp *MiningPool) rpcNegotiatePaymentChannel(conn net.Conn) error {
	fmt.Println("Negotiating payment channel (pool)")

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
	uc := types.UnlockConditions{
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
	pc.channelAddress = uc.UnlockHash()

	// Pool also creates, but does no sign a transaction that funds the channel
	// address.
	pc.size = types.NewCurrency64(10e3)
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
	channelOutputID := channelTxn.SiacoinOutputID(0)
	refundUC, err := mp.wallet.NextAddress()
	refundAddr := refundUC.UnlockHash()
	if err != nil {
		return err
	}
	pc.refundTxn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         channelOutputID,
			UnlockConditions: uc, // 2-of-2 sig
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      pc.size,
			UnlockHash: refundAddr,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(channelOutputID),
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

	fmt.Println("Pool completed the payment channel")
	return nil
}

// sendPayment sends 'amount' through the payment channel whose recipient is
// `addr` by creating a new transaction that sends `amount + oldAmount` to
// `addr` and returning it. This new transaction is meant to then be sent to
// the miner off-chain
func (mp *MiningPool) sendPayment(amount types.Currency, addr types.UnlockHash) (types.Transaction, error) {
	return types.Transaction{}, nil
}
