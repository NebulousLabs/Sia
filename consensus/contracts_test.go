package consensus

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
)

// contractTxn funds and returns a transaction with a file contract.
func contractTxn(t *testing.T, s *State) (txn Transaction) {
	// Create the keys and a siacoin output that adds coins to the keys.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}
	spendConditions := SpendConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: ED25519Identifier,
				Key:       pk[:],
			},
		},
	}
	coinAddress := spendConditions.CoinAddress()
	minerPayouts := []SiacoinOutput{
		SiacoinOutput{
			Value:     CalculateCoinbase(s.height() + 1),
			SpendHash: coinAddress,
		},
	}

	// Mine the block that creates the output.
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), minerPayouts, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction that spends the output.
	input := SiacoinInput{
		OutputID:        b.MinerPayoutID(0),
		SpendConditions: spendConditions,
	}
	output := SiacoinOutput{
		Value:     CalculateCoinbase(s.height()) - 12*1000,
		SpendHash: ZeroAddress,
	}
	contract := FileContract{
		FileSize: 4000,
		Start:    s.height() + 3,
		End:      s.height() + 25*1000,
		Payout:   12 * 1000,
	}
	txn = Transaction{
		SiacoinInputs:  []SiacoinInput{input},
		SiacoinOutputs: []SiacoinOutput{output},
		FileContracts:  []FileContract{contract},
	}

	// Sign the transaction.
	sig := TransactionSignature{
		InputID:        input.OutputID,
		CoveredFields:  CoveredFields{WholeTransaction: true},
		PublicKeyIndex: 0,
	}
	txn.Signatures = append(txn.Signatures, sig)
	sigHash := txn.SigHash(0)
	encodedSig, err := crypto.SignBytes(sigHash[:], sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.Signatures[0].Signature = encodedSig[:]
	return
}

// testContractCreation adds a block with a transaction to the state and checks
// that the contract creation mechanisms work.
func testContractCreation(t *testing.T, s *State) {
	txn := contractTxn(t, s)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Check that the contract made it into the state.
	_, exists := s.openContracts[txn.FileContractID(0)]
	if !exists {
		t.Error("file contract not found found in state after being created")
	}
}

// TestContractCreation creates a new state and uses it to call
// testContractCreation.
func TestContractCreation(t *testing.T) {
	s := CreateGenesisState(Timestamp(time.Now().Unix()))
	testContractCreation(t, s)
}
