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
	state  *State
	tester *testing.T

	unlockConditions UnlockConditions
	unlockHash       UnlockHash
	secretKey        crypto.SecretKey
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
	uc := UnlockConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: SignatureEd25519,
				Key:       encoding.Marshal(pk),
			},
		},
	}
	return &assistant{
		state:            s,
		tester:           t,
		unlockConditions: uc,
		unlockHash:       uc.UnlockHash(),
		secretKey:        sk,
	}
}

// mineCurrentBlock is a shortcut function that calls mineTestingBlock using
// variables that satisfy the current state.
func (a *assistant) mineCurrentBlock(minerPayouts []SiacoinOutput, txns []Transaction) (b Block, err error) {
	return mineTestingBlock(a.state.CurrentBlock().ID(), currentTime(), minerPayouts, txns, a.state.CurrentTarget())
}

// payouts returns a list of payouts that are valid for the given height and
// miner fee total.
func (a *assistant) payouts(height BlockHeight, feeTotal Currency) (payouts []SiacoinOutput) {
	// Get the total miner subsidy.
	valueRemaining := CalculateCoinbase(height).Add(feeTotal)

	// Create several payouts that the assistant can spend, then append a
	// 'remainder' payout.
	for i := 0; i < 12; i++ {
		valueRemaining = valueRemaining.Sub(NewCurrency64(1e6))
		payouts = append(payouts, SiacoinOutput{Value: NewCurrency64(1e6), UnlockHash: a.unlockHash})
	}
	payouts = append(payouts, SiacoinOutput{Value: valueRemaining, UnlockHash: a.unlockHash})

	return
}

// mineValidBlock mines a block and sets a handful of payouts to addresses that
// the assistant can spend, which will give the assistant a good volume of
// outputs to draw on for testing.
func (a *assistant) mineAndApplyValidBlock() (block Block) {
	// Mine the block.
	block, err := mineTestingBlock(a.state.CurrentBlock().ID(), currentTime(), a.payouts(a.state.Height()+1, ZeroCurrency), nil, a.state.CurrentTarget())
	if err != nil {
		a.tester.Fatal(err)
	}

	// Submit the block to the state.
	err = a.state.AcceptBlock(block)
	if err != nil {
		a.tester.Fatal(err)
	}

	return
}

// newTestingEnvironment creates a state and an assistant that wraps around the
// state, then mines enough blocks that the assistant has outputs ready to
// spend.
func newTestingEnvironment(t *testing.T) (a *assistant) {
	// Get the state and assistant.
	s := CreateGenesisState()
	a = newAssistant(t, s)

	// Mine enough blocks that the first miner payouts come to maturity. The
	// assistant will then be ready to spend at least a few outputs.
	for i := 0; i <= MaturityDelay; i++ {
		a.mineAndApplyValidBlock()
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
