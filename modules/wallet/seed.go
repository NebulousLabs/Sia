package wallet

import (
	"bytes"
	"crypto/rand"
	"errors"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	seedFilePrefix = "Sia Wallet Encrypted Backup Seed - "
	seedFileSuffix = ".seed"
)

var (
	errAddressExhaustion = errors.New("current seed has used all available addresses")
	errKnownSeed         = errors.New("seed is already known")
)

type (
	// uniqueID is a unique id randomly generated and put at the front of every
	// persistence object. It is used to make sure that a different encryption
	// key can be used for every persistence object.
	uniqueID [crypto.EntropySize]byte

	// seedFile stores an encrypted wallet seed on disk.
	seedFile struct {
		UID                    uniqueID
		EncryptionVerification crypto.Ciphertext
		Seed                   crypto.Ciphertext
	}
)

// generateSpendableKey creates the keys and unlock conditions for seed at a
// given index.
func generateSpendableKey(seed modules.Seed, index uint64) spendableKey {
	sk, pk := crypto.GenerateKeyPairDeterministic(crypto.HashAll(seed, index))
	return spendableKey{
		UnlockConditions: types.UnlockConditions{
			PublicKeys:         []types.SiaPublicKey{types.Ed25519PublicKey(pk)},
			SignaturesRequired: 1,
		},
		SecretKeys: []crypto.SecretKey{sk},
	}
}

// encryptAndSaveSeedFile encrypts and saves a seed file.
func (w *Wallet) encryptAndSaveSeedFile(masterKey crypto.TwofishKey, seed modules.Seed) (seedFile, error) {
	var sf seedFile
	_, err := rand.Read(sf.UID[:])
	if err != nil {
		return seedFile{}, err
	}
	sek := uidEncryptionKey(masterKey, sf.UID)
	plaintextVerification := make([]byte, encryptionVerificationLen)
	sf.EncryptionVerification, err = sek.EncryptBytes(plaintextVerification)
	if err != nil {
		return seedFile{}, err
	}
	sf.Seed, err = sek.EncryptBytes(seed[:])
	if err != nil {
		return seedFile{}, err
	}
	seedFilename := filepath.Join(w.persistDir, seedFilePrefix+persist.RandomSuffix()+seedFileSuffix)
	err = persist.SaveFileSync(seedMetadata, sf, seedFilename)
	if err != nil {
		return seedFile{}, err
	}
	return sf, nil
}

// decryptSeedFile decrypts a seed file using the encryption key.
func decryptSeedFile(masterKey crypto.TwofishKey, sf seedFile) (seed modules.Seed, err error) {
	// Verify that the provided master key is the correct key.
	decryptionKey := uidEncryptionKey(masterKey, sf.UID)
	expectedDecryptedVerification := make([]byte, crypto.EntropySize)
	decryptedVerification, err := decryptionKey.DecryptBytes(sf.EncryptionVerification)
	if err != nil {
		return modules.Seed{}, err
	}
	if !bytes.Equal(expectedDecryptedVerification, decryptedVerification) {
		return modules.Seed{}, modules.ErrBadEncryptionKey
	}

	// Decrypt and return the seed.
	plainSeed, err := decryptionKey.DecryptBytes(sf.Seed)
	if err != nil {
		return modules.Seed{}, err
	}
	copy(seed[:], plainSeed)
	return seed, nil
}

// integrateSeed takes an address seed as input and from that generates
// 'publicKeysPerSeed' addresses that the wallet is able to spend.
// integrateSeed should not be called with the primary seed.
func (w *Wallet) integrateSeed(seed modules.Seed) {
	for i := uint64(0); i < modules.PublicKeysPerSeed; i++ {
		// Generate the key and check it is new to the wallet.
		spendableKey := generateSpendableKey(seed, i)
		w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	}
	w.seeds = append(w.seeds, seed)
}

// recoverSeed integrates a recovery seed into the wallet.
func (w *Wallet) recoverSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Because the recovery seed does not have a UID, duplication must be
	// prevented by comparing with the list of decrypted seeds. This can only
	// occur while the wallet is unlocked.
	if !w.unlocked {
		return modules.ErrLockedWallet
	}

	// Check that the seed is not already known.
	for _, wSeed := range w.seeds {
		if seed == wSeed {
			return errKnownSeed
		}
	}
	if seed == w.primarySeed {
		return errKnownSeed
	}
	seedFile, err := w.encryptAndSaveSeedFile(masterKey, seed)
	if err != nil {
		return err
	}

	// Add the seed file to the wallet's set of tracked seeds and save the
	// wallet settings.
	w.persist.AuxiliarySeedFiles = append(w.persist.AuxiliarySeedFiles, seedFile)
	err = w.saveSettingsSync()
	if err != nil {
		return err
	}
	w.integrateSeed(seed)
	return nil

}

// createSeed creates a wallet seed and encrypts it using a key derived from
// the master key, then addds it to the wallet as the primary seed, while
// making a disk backup.
func (w *Wallet) createSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	seedFile, err := w.encryptAndSaveSeedFile(masterKey, seed)
	if err != nil {
		return err
	}
	w.primarySeed = seed
	w.persist.PrimarySeedFile = seedFile
	w.persist.PrimarySeedProgress = 0
	// The wallet preloads keys to prevent confusion for people using the same
	// seed/wallet file in multiple places.
	for i := uint64(0); i < modules.WalletSeedPreloadDepth; i++ {
		spendableKey := generateSpendableKey(seed, i)
		w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	}
	return w.saveSettingsSync()
}

// initPrimarySeed loads the primary seed into the wallet.
func (w *Wallet) initPrimarySeed(masterKey crypto.TwofishKey) error {
	seed, err := decryptSeedFile(masterKey, w.persist.PrimarySeedFile)
	if err != nil {
		return err
	}
	// The wallet preloads keys to prevent confusion when using the same wallet
	// in multiple places.
	for i := uint64(0); i < w.persist.PrimarySeedProgress+modules.WalletSeedPreloadDepth; i++ {
		spendableKey := generateSpendableKey(seed, i)
		w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	}
	w.primarySeed = seed
	w.seeds = append(w.seeds, seed)
	return nil
}

// initAuxiliarySeeds scans the wallet folder for wallet seeds.
func (w *Wallet) initAuxiliarySeeds(masterKey crypto.TwofishKey) error {
	for _, seedFile := range w.persist.AuxiliarySeedFiles {
		seed, err := decryptSeedFile(masterKey, seedFile)
		if build.DEBUG && err != nil {
			panic(err)
		}
		if err != nil {
			w.log.Println("UNLOCK: failed to load an auxiliary seed:", err)
			continue
		}
		w.integrateSeed(seed)
	}
	return nil
}

// nextPrimarySeedAddress fetches the next address from the primary seed.
func (w *Wallet) nextPrimarySeedAddress() (types.UnlockConditions, error) {
	// Check that the wallet has been unlocked.
	if !w.unlocked {
		return types.UnlockConditions{}, modules.ErrLockedWallet
	}

	// Integrate the next key into the wallet, and return the unlock
	// conditions. Because the wallet preloads keys, the progress used is
	// 'PrimarySeedProgress+modules.WalletSeedPreloadDepth'.
	spendableKey := generateSpendableKey(w.primarySeed, w.persist.PrimarySeedProgress+modules.WalletSeedPreloadDepth)
	w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	w.persist.PrimarySeedProgress++
	err := w.saveSettingsSync()
	if err != nil {
		return types.UnlockConditions{}, err
	}
	return spendableKey.UnlockConditions, nil
}

// AllSeeds returns a list of all seeds known to and used by the wallet.
func (w *Wallet) AllSeeds() ([]modules.Seed, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return nil, modules.ErrLockedWallet
	}
	return w.seeds, nil
}

// PrimarySeed returns the decrypted primary seed of the wallet.
func (w *Wallet) PrimarySeed() (modules.Seed, uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return modules.Seed{}, 0, modules.ErrLockedWallet
	}
	return w.primarySeed, w.persist.PrimarySeedProgress, nil
}

// NextAddress returns an unlock hash that is ready to receive siacoins or
// siafunds. The address is generated using the primary address seed.
func (w *Wallet) NextAddress() (types.UnlockConditions, error) {
	if err := w.tg.Add(); err != nil {
		return types.UnlockConditions{}, err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nextPrimarySeedAddress()
}

// LoadSeed will track all of the addresses generated by the input seed,
// reclaiming any funds that were lost due to a deleted file or lost encryption
// key. An error will be returned if the seed has already been integrated with
// the wallet.
func (w *Wallet) LoadSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	if err := w.tg.Add(); err != nil {
		return err
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.checkMasterKey(masterKey)
	if err != nil {
		return err
	}
	return w.recoverSeed(masterKey, seed)
}
