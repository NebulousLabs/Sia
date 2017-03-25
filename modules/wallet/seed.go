package wallet

import (
	"errors"
	"runtime"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/fastrand"
)

var (
	errKnownSeed = errors.New("seed is already known")
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

// generateKeys generates n keys from seed, starting from index start.
func generateKeys(seed modules.Seed, start, n uint64) []spendableKey {
	// generate in parallel, one goroutine per core.
	keys := make([]spendableKey, n)
	var wg sync.WaitGroup
	wg.Add(runtime.NumCPU())
	for cpu := 0; cpu < runtime.NumCPU(); cpu++ {
		go func(offset uint64) {
			defer wg.Done()
			for i := offset; i < n; i += uint64(runtime.NumCPU()) {
				// NOTE: don't bother trying to optimize generateSpendableKey;
				// profiling shows that ed25519 key generation consumes far
				// more CPU time than encoding or hashing.
				keys[i] = generateSpendableKey(seed, start+i)
			}
		}(uint64(cpu))
	}
	wg.Wait()
	return keys
}

// createSeedFile creates and encrypts a seedFile.
func createSeedFile(masterKey crypto.TwofishKey, seed modules.Seed) seedFile {
	var sf seedFile
	fastrand.Read(sf.UID[:])
	sek := uidEncryptionKey(masterKey, sf.UID)
	sf.EncryptionVerification = sek.EncryptBytes(verificationPlaintext)
	sf.Seed = sek.EncryptBytes(seed[:])
	return sf
}

// decryptSeedFile decrypts a seed file using the encryption key.
func decryptSeedFile(masterKey crypto.TwofishKey, sf seedFile) (seed modules.Seed, err error) {
	// Verify that the provided master key is the correct key.
	decryptionKey := uidEncryptionKey(masterKey, sf.UID)
	err = verifyEncryption(decryptionKey, sf.EncryptionVerification)
	if err != nil {
		return modules.Seed{}, err
	}

	// Decrypt and return the seed.
	plainSeed, err := decryptionKey.DecryptBytes(sf.Seed)
	if err != nil {
		return modules.Seed{}, err
	}
	copy(seed[:], plainSeed)
	return seed, nil
}

// integrateSeed generates n spendableKeys from the seed and loads them into
// the wallet.
func (w *Wallet) integrateSeed(seed modules.Seed, n uint64) {
	for _, sk := range generateKeys(seed, 0, n) {
		w.keys[sk.UnlockConditions.UnlockHash()] = sk
	}
}

// loadSeed integrates a recovery seed into the wallet.
func (w *Wallet) loadSeed(masterKey crypto.TwofishKey, seed modules.Seed) error {
	// Because the recovery seed does not have a UID, duplication must be
	// prevented by comparing with the list of decrypted seeds. This can only
	// occur while the wallet is unlocked.
	if !w.unlocked {
		return modules.ErrLockedWallet
	}
	if seed == w.primarySeed {
		return errKnownSeed
	}
	for _, wSeed := range w.seeds {
		if seed == wSeed {
			return errKnownSeed
		}
	}

	err := w.db.Update(func(tx *bolt.Tx) error {
		err := checkMasterKey(tx, masterKey)
		if err != nil {
			return err
		}

		// create a seedFile for the seed
		sf := createSeedFile(masterKey, seed)

		// add the seedFile
		var current []seedFile
		err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyAuxiliarySeedFiles), &current)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketWallet).Put(keyAuxiliarySeedFiles, encoding.Marshal(append(current, sf)))
	})
	if err != nil {
		return err
	}

	// load the seed's keys
	w.integrateSeed(seed, modules.PublicKeysPerSeed)
	w.seeds = append(w.seeds, seed)
	return nil
}

// nextPrimarySeedAddress fetches the next address from the primary seed.
func (w *Wallet) nextPrimarySeedAddress(tx *bolt.Tx) (types.UnlockConditions, error) {
	// Check that the wallet has been unlocked.
	if !w.unlocked {
		return types.UnlockConditions{}, modules.ErrLockedWallet
	}

	// Fetch and increment the seed progress.
	progress, err := dbGetPrimarySeedProgress(tx)
	if err != nil {
		return types.UnlockConditions{}, err
	}
	if err = dbPutPrimarySeedProgress(tx, progress+1); err != nil {
		return types.UnlockConditions{}, err
	}
	// Integrate the next key into the wallet, and return the unlock
	// conditions.
	spendableKey := generateSpendableKey(w.primarySeed, progress)
	w.keys[spendableKey.UnlockConditions.UnlockHash()] = spendableKey
	return spendableKey.UnlockConditions, nil
}

// AllSeeds returns a list of all seeds known to and used by the wallet.
func (w *Wallet) AllSeeds() ([]modules.Seed, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return nil, modules.ErrLockedWallet
	}
	return append([]modules.Seed{w.primarySeed}, w.seeds...), nil
}

// PrimarySeed returns the decrypted primary seed of the wallet, as well as
// the number of addresses that the seed can be safely used to generate.
func (w *Wallet) PrimarySeed() (modules.Seed, uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.unlocked {
		return modules.Seed{}, 0, modules.ErrLockedWallet
	}
	// TODO: going to the db is slow; consider caching progress. On the other
	// hand, PrimarySeed isn't a frequently called method, so caching may be
	// overkill.
	var progress uint64
	err := w.db.View(func(tx *bolt.Tx) error {
		var err error
		progress, err = dbGetPrimarySeedProgress(tx)
		return err
	})
	if err != nil {
		return modules.Seed{}, 0, err
	}

	// addresses remaining is maxScanKeys-progress; generating more keys than
	// that risks not being able to recover them when using SweepSeed or
	// InitFromSeed.
	remaining := maxScanKeys - progress
	if progress > maxScanKeys {
		remaining = 0
	}
	return w.primarySeed, remaining, nil
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

	// TODO: going to the db is slow; consider creating 100 addresses at a
	// time.
	var uc types.UnlockConditions
	err := w.db.Update(func(tx *bolt.Tx) error {
		var err error
		uc, err = w.nextPrimarySeedAddress(tx)
		return err
	})
	return uc, err
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
	return w.loadSeed(masterKey, seed)
}

// SweepSeed scans the blockchain for outputs generated from seed and creates
// a transaction that transfers them to the wallet. Note that this incurs a
// transaction fee. It returns the total value of the outputs, minus the fee.
// If only siafunds were found, the fee is deducted from the wallet.
func (w *Wallet) SweepSeed(seed modules.Seed) (coins, funds types.Currency, err error) {
	if err = w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	w.mu.RLock()
	match := seed == w.primarySeed
	w.mu.RUnlock()
	if match {
		return types.Currency{}, types.Currency{}, errors.New("cannot sweep primary seed")
	}

	if !w.cs.Synced() {
		return types.Currency{}, types.Currency{}, errors.New("cannot sweep until blockchain is synced")
	}

	// get an address to spend into
	var uc types.UnlockConditions
	err = w.db.Update(func(tx *bolt.Tx) error {
		var err error
		uc, err = w.nextPrimarySeedAddress(tx)
		return err
	})
	if err != nil {
		return
	}

	// scan blockchain for outputs, filtering out 'dust' (outputs that cost
	// more in fees than they are worth)
	s := newSeedScanner(seed)
	_, maxFee := w.tpool.FeeEstimation()
	const outputSize = 350 // approx. size in bytes of an output and accompanying signature
	s.dustThreshold = maxFee.Mul64(outputSize)
	if err = s.scan(w.cs); err != nil {
		return
	}

	// construct a transaction that spends the outputs
	// TODO: this may result in transactions that are too large.
	tb := w.StartTransaction()
	var sweptCoins, sweptFunds types.Currency // total values of swept outputs
	for _, output := range s.siacoinOutputs {
		// construct a siacoin input that spends the output
		sk := generateSpendableKey(seed, output.seedIndex)
		tb.AddSiacoinInput(types.SiacoinInput{
			ParentID:         types.SiacoinOutputID(output.id),
			UnlockConditions: sk.UnlockConditions,
		})
		// add a signature for the input
		sweptCoins = sweptCoins.Add(output.value)
	}
	for _, output := range s.siafundOutputs {
		// construct a siafund input that spends the output
		sk := generateSpendableKey(seed, output.seedIndex)
		tb.AddSiafundInput(types.SiafundInput{
			ParentID:         types.SiafundOutputID(output.id),
			UnlockConditions: sk.UnlockConditions,
		})
		// add a signature for the input
		sweptFunds = sweptFunds.Add(output.value)
	}

	// estimate the transaction size and fee. NOTE: this equation doesn't
	// account for other fields in the transaction, but since we are
	// multiplying by maxFee, lowballing is ok
	estTxnSize := (len(s.siacoinOutputs) + len(s.siafundOutputs)) * outputSize
	estFee := maxFee.Mul64(uint64(estTxnSize))
	tb.AddMinerFee(estFee)

	// calculate total siacoin payout
	if sweptCoins.Cmp(estFee) > 0 {
		coins = sweptCoins.Sub(estFee)
	}
	funds = sweptFunds

	switch {
	case coins.IsZero() && funds.IsZero():
		// if we aren't sweeping any coins or funds, then just return an
		// error; no reason to proceed
		return types.Currency{}, types.Currency{}, errors.New("transaction fee exceeds value of swept outputs")

	case !coins.IsZero() && funds.IsZero():
		// if we're sweeping coins but not funds, add a siacoin output for
		// them
		tb.AddSiacoinOutput(types.SiacoinOutput{
			Value:      coins,
			UnlockHash: uc.UnlockHash(),
		})

	case coins.IsZero() && !funds.IsZero():
		// if we're sweeping funds but not coins, add a siafund output for
		// them. This is tricky because we still need to pay for the
		// transaction fee, but we can't simply subtract the fee from the
		// output value like we can with swept coins. Instead, we need to fund
		// the fee using the existing wallet balance.
		tb.AddSiafundOutput(types.SiafundOutput{
			Value:      funds,
			UnlockHash: uc.UnlockHash(),
		})
		err = tb.FundSiacoins(estFee)
		if err != nil {
			return types.Currency{}, types.Currency{}, errors.New("couldn't pay transaction fee on swept funds: " + err.Error())
		}

	case !coins.IsZero() && !funds.IsZero():
		// if we're sweeping both coins and funds, add a siacoin output and a
		// siafund output
		tb.AddSiacoinOutput(types.SiacoinOutput{
			Value:      coins,
			UnlockHash: uc.UnlockHash(),
		})
		tb.AddSiafundOutput(types.SiafundOutput{
			Value:      funds,
			UnlockHash: uc.UnlockHash(),
		})
	}

	// add signatures for all coins and funds (manually, since tb doesn't have
	// access to the signing keys)
	txn, parents := tb.View()
	for _, output := range s.siacoinOutputs {
		sk := generateSpendableKey(seed, output.seedIndex)
		addSignatures(&txn, types.FullCoveredFields, sk.UnlockConditions, crypto.Hash(output.id), sk)
	}
	for _, sfo := range s.siafundOutputs {
		sk := generateSpendableKey(seed, sfo.seedIndex)
		addSignatures(&txn, types.FullCoveredFields, sk.UnlockConditions, crypto.Hash(sfo.id), sk)
	}
	// Usually, all the inputs will come from swept outputs. However, there is
	// an edge case in which inputs will be added from the wallet. To cover
	// this case, we iterate through the SiacoinInputs and add a signature for
	// any input that belongs to the wallet.
	w.mu.RLock()
	for _, input := range txn.SiacoinInputs {
		if key, ok := w.keys[input.UnlockConditions.UnlockHash()]; ok {
			addSignatures(&txn, types.FullCoveredFields, input.UnlockConditions, crypto.Hash(input.ParentID), key)
		}
	}
	w.mu.RUnlock()

	// submit the transaction
	txnSet := append(parents, txn)
	err = w.tpool.AcceptTransactionSet(txnSet)
	return
}
