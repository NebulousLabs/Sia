package consensus

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// An Assistant keeps track of addresses and contracts and whatnot to help with
// testing. There are also helper functions for mining blocks and cobbling
// together transactions. It's designed to be simple, and it's not very smart
// or efficient.
type Assistant struct {
	State  *State
	Tester *testing.T

	UnlockConditions UnlockConditions
	UnlockHash       UnlockHash
	SecretKey        crypto.SecretKey

	usedOutputs map[SiacoinOutputID]struct{}
}

// CurrentTime returns a Timestamp of the current time.
func CurrentTime() Timestamp {
	return Timestamp(time.Now().Unix())
}

// MineTestingBlock accepts a bunch of parameters for a block and then grinds
// blocks until a block with the appropriate target is found.
func MineTestingBlock(parent BlockID, timestamp Timestamp, minerPayouts []SiacoinOutput, txns []Transaction, target Target) (b Block, err error) {
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

// MineCurrentBlock is a shortcut function that calls MineTestingBlock using
// variables that satisfy the current state.
func (a *Assistant) MineCurrentBlock(txns []Transaction) (b Block, err error) {
	minerPayouts := a.Payouts(a.State.Height()+1, txns)
	return MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTime(), minerPayouts, txns, a.State.CurrentTarget())
}

// Payouts returns a block with 12 payouts worth 1e6 and a final payout that
// makes the total payout amount add up correctly. This produces a large set of
// outputs that can be used for testing.
func (a *Assistant) Payouts(height BlockHeight, txns []Transaction) (payouts []SiacoinOutput) {
	var feeTotal Currency
	for _, txn := range txns {
		for _, fee := range txn.MinerFees {
			feeTotal = feeTotal.Add(fee)
		}
	}

	// Get the total miner subsidy.
	valueRemaining := CalculateCoinbase(height).Add(feeTotal)

	// Create several payouts that the assistant can spend, then append a
	// 'remainder' payout.
	for i := 0; i < 12; i++ {
		valueRemaining = valueRemaining.Sub(NewCurrency64(1e6))
		payouts = append(payouts, SiacoinOutput{Value: NewCurrency64(1e6), UnlockHash: a.UnlockHash})
	}
	payouts = append(payouts, SiacoinOutput{Value: valueRemaining, UnlockHash: a.UnlockHash})

	return
}

// MineAndApplyValidBlock mines a block and sets a handful of payouts to
// addresses that the assistant can spend, which will give the assistant a good
// volume of outputs to draw on for testing.
func (a *Assistant) MineAndApplyValidBlock() (block Block) {
	// Mine the block.
	block, err := MineTestingBlock(a.State.CurrentBlock().ID(), CurrentTime(), a.Payouts(a.State.Height()+1, nil), nil, a.State.CurrentTarget())
	if err != nil {
		a.Tester.Fatal(err)
	}

	// Submit the block to the state.
	err = a.State.AcceptBlock(block)
	if err != nil {
		a.Tester.Fatal(err)
	}

	return
}

// RewindABlock removes the most recent block from the consensus set.
func (a *Assistant) RewindABlock() {
	bn := a.State.currentBlockNode()
	direction := false // set to false because we're removing a block.
	a.State.applyDiffSet(bn, direction)
}

// NewAssistant returns an assistant that's ready to help with testing.
func NewAssistant(t *testing.T, s *State) *Assistant {
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
	return &Assistant{
		State:            s,
		Tester:           t,
		UnlockConditions: uc,
		UnlockHash:       uc.UnlockHash(),
		SecretKey:        sk,

		usedOutputs: make(map[SiacoinOutputID]struct{}),
	}
}

// NewTestingEnvironment creates a state and an assistant that wraps around the
// state, then mines enough blocks that the assistant has outputs ready to
// spend.
func NewTestingEnvironment(t *testing.T) (a *Assistant) {
	// Get the state and assistant.
	s := CreateGenesisState()
	a = NewAssistant(t, s)

	// Mine enough blocks that the first miner payouts come to maturity. The
	// assistant will then be ready to spend at least a few outputs.
	for i := 0; i <= MaturityDelay; i++ {
		a.MineAndApplyValidBlock()
	}

	return
}
