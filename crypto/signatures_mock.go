package crypto

import (
	"crypto/rand"
	"io"

	"github.com/NebulousLabs/ed25519"
)

type (
	// keyDeriver contains all of the dependencies for a signature key
	// generator. The dependencies are separated to enable mocking.
	keyDeriver interface {
		deriveKeyPair([EntropySize]byte) (ed25519.SecretKey, ed25519.PublicKey)
	}
)

var (
	// stdKeyGen is a signature generator that can be used to generate random
	// and deterministic keys for signing objects.
	stdKeyGen sigKeyGen = sigKeyGen{entropySource: rand.Reader, keyDeriver: &stdKeyDeriver{}}
)

// sigKeyGen contains a set of dependencies that are used to build out the core
// logic for generating keys in Sia.
type sigKeyGen struct {
	entropySource io.Reader
	keyDeriver    keyDeriver
}

// generate builds a signature keypair using a sigKeyGen to manage
// dependencies.
func (skg sigKeyGen) generate() (sk SecretKey, pk PublicKey, err error) {
	var entropy [EntropySize]byte
	_, err = skg.entropySource.Read(entropy[:])
	if err != nil {
		return
	}

	skPointer, pkPointer := skg.keyDeriver.deriveKeyPair(entropy)
	return *skPointer, *pkPointer, nil
}

// generateDeterministic builds a signature keypair deterministically using a
// sigKeyGen to manage dependencies.
func (skg sigKeyGen) generateDeterministic(entropy [EntropySize]byte) (SecretKey, PublicKey) {
	skPointer, pkPointer := skg.keyDeriver.deriveKeyPair(entropy)
	return *skPointer, *pkPointer
}

// stdKeyDeriver implements the keyDeriver dependency for the sigKeyGen.
type stdKeyDeriver struct{}

func (skd *stdKeyDeriver) deriveKeyPair(entropy [EntropySize]byte) (ed25519.SecretKey, ed25519.PublicKey) {
	return ed25519.GenerateKey(entropy)
}
