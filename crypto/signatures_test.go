package crypto

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/ed25519"
)

// mockRandGenerator is a mock implementation of readBytesFunc, allowing tests
// to mock out calls to rand.Read and use a deterministic implementation
// instead.
type mockRandGenerator struct {
	mock.Mock
}

func (m *mockRandGenerator) Read(b []byte) (n int, err error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

// mockKeyDeriver is a mock implementation of deriveEd25519KeyPairFunc, allowing
// tests to mock out calls to ed25519.GenerateKey.
type mockKeyDeriver struct {
	mock.Mock
}

func (m *mockKeyDeriver) Derive(b [EntropySize]byte) (sk ed25519.SecretKey, pk ed25519.PublicKey) {
	args := m.Called(b)
	return args.Get(0).(ed25519.SecretKey), args.Get(1).(ed25519.PublicKey)
}

// Test that the Generate method is properly calling its dependencies and
// returning the expected key pair.
func TestGenerateRandomKeyPair(t *testing.T) {
	// Create a mock random number generator (note that it does not actually
	// modify its buffer.
	mockRandGenerator := new(mockRandGenerator)
	mockRandGenerator.On("Read", mock.Anything).Return(EntropySize, nil)

	derivedSk := ed25519.SecretKey(new([SecretKeySize]byte))
	derivedPk := ed25519.PublicKey(new([PublicKeySize]byte))
	mockEd22519 := new(mockKeyDeriver)
	mockEd22519.On("Derive", *new([EntropySize]byte)).Return(derivedSk, derivedPk)

	// Create a SignatureKeyGenerator using mocks.
	skg := SignatureKeyGenerator{mockRandGenerator.Read, mockEd22519.Derive}

	// Create key pair.
	skActual, pkActual, err := skg.Generate()

	// Verify that Generate satisfied expectations.
	assert.Nil(t, err)
	assert.Equal(t, SecretKey(*derivedSk), skActual)
	assert.Equal(t, PublicKey(*derivedPk), pkActual)

	mockRandGenerator.AssertExpectations(t)
	mockEd22519.AssertExpectations(t)
}

// Test that the Generate method fails if the call to readRandBytes fails.
func TestGenerateRandomKeyPairFailsWhenRandFails(t *testing.T) {
	// Create a mock random number generator that fails when called.
	mockRandGenerator := new(mockRandGenerator)
	mockRandGenerator.On("Read", mock.Anything).Return(EntropySize, errors.New("mock error from readRandBytes"))

	derivedSk := ed25519.SecretKey(new([SecretKeySize]byte))
	derivedPk := ed25519.PublicKey(new([PublicKeySize]byte))
	mockEd22519 := new(mockKeyDeriver)
	mockEd22519.On("Derive", *new([EntropySize]byte)).Return(derivedSk, derivedPk)

	skg := SignatureKeyGenerator{mockRandGenerator.Read, mockEd22519.Derive}
	_, _, err := skg.Generate()
	assert.NotNil(t, err, "Generate should fail when readRandBytes fails")
}

// Test that the Generate method fails if readRandBytes does not fill the
// buffer.
func TestGenerateRandomKeyPairFailsWhenRandWritesInsufficientBytes(t *testing.T) {
	// Create a mock random number generator that succeeds but reports that it
	// populated 1 less byte than the buffer size.
	mockRandGenerator := new(mockRandGenerator)
	mockRandGenerator.On("Read", mock.Anything).Return(EntropySize-1, nil)

	derivedSk := ed25519.SecretKey(new([SecretKeySize]byte))
	derivedPk := ed25519.PublicKey(new([PublicKeySize]byte))
	mockEd22519 := new(mockKeyDeriver)
	mockEd22519.On("Derive", *new([EntropySize]byte)).Return(derivedSk, derivedPk)

	skg := SignatureKeyGenerator{mockRandGenerator.Read, mockEd22519.Derive}
	_, _, err := skg.Generate()
	assert.Equal(t, ErrRandUnexpected, err, "Generate should fail when readRandBytes writes insufficient bytes")
}

// Test that the Generate method is properly calling its dependencies and
// returning the expected key pair.
func TestGenerateDeterministicKeyPair(t *testing.T) {
	// Create entropy bytes, setting a few bytes explicitly instead of using a
	// buffer of random bytes.
	var entropy [EntropySize]byte
	entropy[0] = 0x05
	entropy[1] = 0x08

	derivedSk := ed25519.SecretKey(new([SecretKeySize]byte))
	derivedPk := ed25519.PublicKey(new([PublicKeySize]byte))
	mockEd22519 := new(mockKeyDeriver)
	mockEd22519.On("Derive", entropy).Return(derivedSk, derivedPk)

	skg := SignatureKeyGenerator{nil, mockEd22519.Derive}

	// Create key pair.
	skActual, pkActual := skg.GenerateDeterministic(entropy)
	assert.Equal(t, SecretKey(*derivedSk), skActual)
	assert.Equal(t, PublicKey(*derivedPk), pkActual)

	mockEd22519.AssertExpectations(t)
}

// Creates and encodes a public key, and verifies that it decodes correctly,
// does the same with a signature.
func TestSignatureEncoding(t *testing.T) {
	assert := assert.New(t)
	// Create a dummy key pair.
	var sk SecretKey
	sk[0] = 0x0a
	sk[32] = 0x0b
	pk := sk.PublicKey()

	// Marshal and unmarshal the public key.
	marshalledPK := encoding.Marshal(pk)
	var unmarshalledPK PublicKey
	err := encoding.Unmarshal(marshalledPK, &unmarshalledPK)
	assert.Nil(err)

	// Test the public keys for equality.
	assert.Equal(pk, unmarshalledPK, "pubkey not the same after marshalling and unmarshalling")

	// Create a signature using the secret key.
	var signedData Hash
	rand.Read(signedData[:])
	sig, err := SignHash(signedData, sk)
	assert.Nil(err)

	// Marshal and unmarshal the signature.
	marshalledSig := encoding.Marshal(sig)
	var unmarshalledSig Signature
	err = encoding.Unmarshal(marshalledSig, &unmarshalledSig)
	assert.Nil(err)

	// Test signatures for equality.
	assert.Equal(sig, unmarshalledSig, "signature not same after marshalling and unmarshalling")
}

// TestSigning creates a bunch of keypairs and signs random data with each of
// them.
func TestSigning(t *testing.T) {
	require := require.New(t)
	if testing.Short() {
		t.SkipNow()
	}

	// Try a bunch of signatures because at one point there was a library that
	// worked around 98% of the time. Tests would usually pass, but 200
	// iterations would normally cause a failure.
	iterations := 200
	for i := 0; i < iterations; i++ {
		skg := NewSignatureKeyGenerator()

		// Create dummy key pair.
		var entropy [EntropySize]byte
		entropy[0] = 0x05
		entropy[1] = 0x08
		sk, pk := skg.GenerateDeterministic(entropy)

		// Generate and sign the data.
		var randData Hash
		rand.Read(randData[:])
		sig, err := SignHash(randData, sk)
		require.Nil(err)

		// Verify the signature.
		err = VerifyHash(randData, pk, sig)
		require.Nil(err)

		// Attempt to verify after the data has been altered.
		randData[0] += 1
		err = VerifyHash(randData, pk, sig)
		require.Equal(ErrInvalidSignature, err)

		// Restore the data and make sure the signature is valid again.
		randData[0] -= 1
		err = VerifyHash(randData, pk, sig)
		require.Nil(err)

		// Attempt to verify after the signature has been altered.
		sig[0] += 1
		err = VerifyHash(randData, pk, sig)
		require.Equal(ErrInvalidSignature, err)
	}
}
