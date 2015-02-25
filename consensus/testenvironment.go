package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// TODO: Add MineAndSubmitCurrentBlock, which mines the current block and calls
// accept block, checking for err = nil.

// A ConsensusTester holds a state and a testing object as well as some minimal
// and simplistic features for performing actions such as mining and building
// transactions.
type ConsensusTester struct {
	*State
	*testing.T

	UnlockConditions UnlockConditions
	UnlockHash       UnlockHash
	SecretKey        crypto.SecretKey

	usedOutputs map[SiacoinOutputID]struct{}
}

// MineTestingBlock accepts a bunch of parameters for a block and then grinds
// blocks until a block with the appropriate target is found.
func MineTestingBlock(parent BlockID, timestamp Timestamp, minerPayouts []SiacoinOutput, txns []Transaction, target Target) (b Block) {
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
func (ct *ConsensusTester) MineCurrentBlock(txns []Transaction) (b Block) {
	minerPayouts := ct.Payouts(ct.Height()+1, txns)
	return MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), minerPayouts, txns, ct.CurrentTarget())
}

// MineAndSubmitCurrentBlock is a shortcut function that calls MineCurrentBlock
// and then submits it to the state.
func (ct *ConsensusTester) MineAndSubmitCurrentBlock(txns []Transaction) error {
	minerPayouts := ct.Payouts(ct.Height()+1, txns)
	block := MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), minerPayouts, txns, ct.CurrentTarget())
	return ct.AcceptBlock(block)
}

// Payouts returns a block with 12 payouts worth 1e6 and a final payout that
// makes the total payout amount add up correctly. This produces a large set of
// outputs that can be used for testing.
func (ct *ConsensusTester) Payouts(height BlockHeight, txns []Transaction) (payouts []SiacoinOutput) {
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
		payouts = append(payouts, SiacoinOutput{Value: NewCurrency64(1e6), UnlockHash: ct.UnlockHash})
	}
	payouts = append(payouts, SiacoinOutput{Value: valueRemaining, UnlockHash: ct.UnlockHash})

	return
}

// MineAndApplyValidBlock mines a block and sets a handful of payouts to
// addresses that the assistant can spend, which will give the assistant a good
// volume of outputs to draw on for testing.
func (ct *ConsensusTester) MineAndApplyValidBlock() (block Block) {
	block = MineTestingBlock(ct.CurrentBlock().ID(), CurrentTimestamp(), ct.Payouts(ct.Height()+1, nil), nil, ct.CurrentTarget())
	err := ct.AcceptBlock(block)
	if err != nil {
		ct.Fatal(err)
	}
	return
}

// RewindABlock removes the most recent block from the consensus set.
func (ct *ConsensusTester) RewindABlock() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	bn := ct.currentBlockNode()
	ct.commitDiffSet(bn, DiffRevert)
}

// NewConsensusTester returns an assistant that's ready to help with testing.
func NewConsensusTester(t *testing.T, s *State) (ct *ConsensusTester) {
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}
	uc := UnlockConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: SignatureEd25519,
				Key:       string(encoding.Marshal(pk)),
			},
		},
	}
	ct = &ConsensusTester{
		UnlockConditions: uc,
		UnlockHash:       uc.UnlockHash(),
		SecretKey:        sk,

		usedOutputs: make(map[SiacoinOutputID]struct{}),
	}
	ct.State = s
	ct.T = t
	return
}

// NewTestingEnvironment creates a state and an assistant that wraps around the
// state, then mines enough blocks that the assistant has outputs ready to
// spend.
func NewTestingEnvironment(t *testing.T) (ct *ConsensusTester) {
	// Get the state and assistant.
	s := CreateGenesisState()
	ct = NewConsensusTester(t, s)

	// Mine enough blocks that the first miner payouts come to maturity. The
	// assistant will then be ready to spend at least a few outputs.
	for i := 0; i <= MaturityDelay; i++ {
		ct.MineAndApplyValidBlock()
	}

	return
}
