package wallet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// mockHashSigner serves as a mock replacement for crypto.SignHash.
type mockHashSigner struct {
	mock.Mock
}

func (hs *mockHashSigner) SignHash(hash crypto.Hash, key crypto.SecretKey) (sig crypto.Signature, err error) {
	args := hs.Called(hash, key)
	return args.Get(0).(crypto.Signature), args.Error(1)
}

func generateMockSignatureKeys() (sk crypto.SecretKey, pk crypto.PublicKey) {
	sk[0] = 0x0a
	sk[32] = 0x0b
	return sk, sk.PublicKey()
}

func publicKeyToSiaPublicKey(pk crypto.PublicKey) (spk types.SiaPublicKey) {
	spk.Key = pk[:]
	return spk
}

func TestAddSignaturesOnEmptyTransaction(t *testing.T) {
	sk, pk := generateMockSignatureKeys()
	txn := types.Transaction{}
	cf := types.CoveredFields{}
	uc := types.UnlockConditions{}
	uc.PublicKeys = append(uc.PublicKeys, publicKeyToSiaPublicKey(pk))
	parentID := crypto.Hash{}
	spendKey := spendableKey{}
	spendKey.SecretKeys = append(spendKey.SecretKeys, sk)

	// Create a mock hash signer that always returns a signature of
	// {0x0c, 0x00, 0x00, ...}
	hs := new(mockHashSigner)
	var mockSignature crypto.Signature
	mockSignature[0] = 0x0c
	hs.On("SignHash", mock.Anything, sk).Return(mockSignature, nil)

	addSignaturesWithHashSigner(&txn, cf, uc, parentID, spendKey, hs.SignHash)

	assert.Equal(t, 1, len(txn.TransactionSignatures))
	assert.Equal(t, mockSignature[:], txn.TransactionSignatures[0].Signature)
	hs.AssertExpectations(t)
}
