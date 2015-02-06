package consensus

// consensus_test.go contains objects and functions that help to build an
// architecture for testing. Such objects include a very basic wallet and a
// basic miner.

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// An assistant keeps track of addresses and contracts and whatnot to help with
// testing. There are also helper functions for mining blocks and cobbling
// together transactions. It's designed to be simple, and it's not very smart
// or efficient.
type assistant struct {
	s *State
	t *testing.T

	spendConditions SpendConditions
	coinAddress     CoinAddress
	secretKey       crypto.SecretKey
}

// currentTime returns a Timestamp of the current time.
func currentTime() Timestamp {
	return Timestamp(time.Now().Unix())
}

// mineTestingBlock accepts a bunch of parameters for a block and then grinds
// blocks until a block with the appropriate target is found.
func mineTestingBlock(parent BlockID, timestamp Timestamp, minerPayouts []SiacoinOutput, txns []Transaction, target Target) (b Block, err error) {
	b = Block{
		ParentID:     parent,
		Timestamp:    timestamp,
		MinerPayouts: minerPayouts,
		Transactions: txns,
	}

	for !b.CheckTarget(target) && b.Nonce < 1e6 {
		b.Nonce++
	}
	if !b.CheckTarget(target) {
		panic("mineTestingBlock failed!")
	}
	return
}

// newAssistant returns an assistant that's ready to help with testing.
func newAssistant(t *testing.T, s *State) *assistant {
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}
	sc := SpendConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	return &assistant{
		s:               s,
		t:               t,
		spendConditions: sc,
		coinAddress:     sc.CoinAddress(),
		secretKey:       sk,
	}
}

// mineValidBlock mines a block and sets a handful of payouts to addresses that
// the assistant can spend, which will give the assistant a good volume of
// outputs to draw on for testing.
func (a *assistant) mineValidBlock() {
	// Create the patouts
	var payouts []SiacoinOutput
	valueRemaining := CalculateCoinbase(a.s.height())
	for i := 0; i < 12; i++ {
		err := valueRemaining.Sub(NewCurrency64(1e6))
		if err != nil {
			a.t.Fatal(err)
		}
		payouts = append(payouts, SiacoinOutput{Value: NewCurrency64(1e6), SpendHash: a.coinAddress})
	}
	payouts = append(payouts, SiacoinOutput{Value: valueRemaining, SpendHash: a.coinAddress})

	// Mine the block.
	block, err := mineTestingBlock(a.s.CurrentBlock().ID(), currentTime(), payouts, nil, a.s.CurrentTarget())

	// Submit the block to the state.
	err = a.s.AcceptBlock(block)
	if err != nil {
		a.t.Fatal(err)
	}
}

// newTestingEnvironment creates a state and an assistant that wraps around the
// state, then mines enough blocks that the assistant has outputs ready to
// spend.
func newTestingEnvironment(t *testing.T) (a *assistant) {
	// Get the state and assistant.
	s := CreateGenesisState(currentTime())
	a = newAssistant(t, s)

	// Mine enough blocks that the first miner payouts come to maturity. The
	// assistent will then be ready to spend at least a few outputs.
	for i := 0; i <= MaturityDelay; i++ {
		a.mineValidBlock()
	}

	return
}

// TODO: Deprecate the below functions.

// nullMinerPayouts returns an []Output for the miner payouts field of a block
// so that the block can be valid. It assumes the block will be at whatever
// height you use as input.
func nullMinerPayouts(height BlockHeight) []SiacoinOutput {
	return []SiacoinOutput{
		SiacoinOutput{
			Value: CalculateCoinbase(height),
		},
	}
}

// mineValidBlock picks valid/legal parameters for a block and then uses them
// to call mineTestingBlock.
func mineValidBlock(s *State) (b Block, err error) {
	return mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), nil, s.CurrentTarget())
}
