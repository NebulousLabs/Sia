package wallet

import (
	"encoding/binary"
	"errors"
	"reflect"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"

	"github.com/NebulousLabs/bolt"
)

var (
	// bucketProcessedTransactions stores ProcessedTransactions in
	// chronological order. Only transactions relevant to the wallet are
	// stored. The key of this bucket is an autoincrementing integer.
	bucketProcessedTransactions = []byte("bucketProcessedTransactions")
	// bucketSiacoinOutputs maps a SiacoinOutputID to its SiacoinOutput. Only
	// outputs that the wallet controls are stored. The wallet uses these
	// outputs to fund transactions.
	bucketSiacoinOutputs = []byte("bucketSiacoinOutputs")
	// bucketSiacoinOutputs maps a SiafundOutputID to its SiafundOutput. Only
	// outputs that the wallet controls are stored. The wallet uses these
	// outputs to fund transactions.
	bucketSiafundOutputs = []byte("bucketSiafundOutputs")
	// bucketSpentOutputs maps an OutputID to the height at which it was
	// spent. Only outputs spent by the wallet are stored. The wallet tracks
	// these outputs so that it can reuse them if they are not confirmed on
	// the blockchain.
	bucketSpentOutputs = []byte("bucketSpentOutputs")
	// bucketWallet contains various fields needed by the wallet, such as its
	// UID, EncryptionVerification, and PrimarySeedFile.
	bucketWallet = []byte("bucketWallet")

	dbBuckets = [][]byte{
		bucketProcessedTransactions,
		bucketSiacoinOutputs,
		bucketSiafundOutputs,
		bucketSpentOutputs,
		bucketWallet,
	}

	// these keys are used in bucketWallet
	keyUID                    = []byte("keyUID")
	keyEncryptionVerification = []byte("keyEncryptionVerification")
	keyPrimarySeedFile        = []byte("keyPrimarySeedFile")
	keyPrimarySeedProgress    = []byte("keyPrimarySeedProgress")
	keyConsensusChange        = []byte("keyConsensusChange")
	keyConsensusHeight        = []byte("keyConsensusHeight")
	keySpendableKeyFiles      = []byte("keySpendableKeyFiles")
	keyAuxiliarySeedFiles     = []byte("keyAuxiliarySeedFiles")
	keySiafundPool            = []byte("keySiafundPool")

	errNoKey = errors.New("key does not exist")
)

// threadedDBUpdate commits the active database transaction and starts a new
// transaction.
func (w *Wallet) threadedDBUpdate() {
	if err := w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	for {
		select {
		case <-time.After(2 * time.Minute):
		case <-w.tg.StopChan():
			return
		}
		w.mu.Lock()
		w.syncDB()
		w.mu.Unlock()
	}
}

// syncDB commits the current global transaction and immediately begins a
// new one. It must be called with a write-lock.
func (w *Wallet) syncDB() {
	// commit the current tx
	err := w.dbTx.Commit()
	if err != nil {
		w.log.Severe("ERROR: failed to apply database update:", err)
		w.dbTx.Rollback()
	}
	// begin a new tx
	w.dbTx, err = w.db.Begin(true)
	if err != nil {
		w.log.Severe("ERROR: failed to start database update:", err)
	}
}

// dbReset wipes and reinitializes a wallet database.
func dbReset(tx *bolt.Tx) error {
	for _, bucket := range dbBuckets {
		err := tx.DeleteBucket(bucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucket(bucket)
		if err != nil {
			return err
		}
	}

	// reinitialize the database with default values
	wb := tx.Bucket(bucketWallet)
	wb.Put(keyUID, fastrand.Bytes(len(uniqueID{})))
	wb.Put(keyConsensusHeight, encoding.Marshal(uint64(0)))
	wb.Put(keyAuxiliarySeedFiles, encoding.Marshal([]seedFile{}))
	wb.Put(keySpendableKeyFiles, encoding.Marshal([]spendableKeyFile{}))
	dbPutConsensusHeight(tx, 0)
	dbPutConsensusChangeID(tx, modules.ConsensusChangeBeginning)
	dbPutSiafundPool(tx, types.ZeroCurrency)

	return nil
}

// dbPut is a helper function for storing a marshalled key/value pair.
func dbPut(b *bolt.Bucket, key, val interface{}) error {
	return b.Put(encoding.Marshal(key), encoding.Marshal(val))
}

// dbGet is a helper function for retrieving a marshalled key/value pair. val
// must be a pointer.
func dbGet(b *bolt.Bucket, key, val interface{}) error {
	valBytes := b.Get(encoding.Marshal(key))
	if valBytes == nil {
		return errNoKey
	}
	return encoding.Unmarshal(valBytes, val)
}

// dbDelete is a helper function for deleting a marshalled key/value pair.
func dbDelete(b *bolt.Bucket, key interface{}) error {
	return b.Delete(encoding.Marshal(key))
}

// dbForEach is a helper function for iterating over a bucket and calling fn
// on each entry. fn must be a function with two parameters. The key/value
// bytes of each bucket entry will be unmarshalled into the types of fn's
// parameters.
func dbForEach(b *bolt.Bucket, fn interface{}) error {
	// check function type
	fnVal, fnTyp := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if fnTyp.Kind() != reflect.Func || fnTyp.NumIn() != 2 {
		panic("bad fn type: needed func(key, val), got " + fnTyp.String())
	}

	return b.ForEach(func(keyBytes, valBytes []byte) error {
		key, val := reflect.New(fnTyp.In(0)), reflect.New(fnTyp.In(1))
		if err := encoding.Unmarshal(keyBytes, key.Interface()); err != nil {
			return err
		} else if err := encoding.Unmarshal(valBytes, val.Interface()); err != nil {
			return err
		}
		fnVal.Call([]reflect.Value{key.Elem(), val.Elem()})
		return nil
	})
}

// Type-safe wrappers around the db helpers

func dbPutSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID, output types.SiacoinOutput) error {
	return dbPut(tx.Bucket(bucketSiacoinOutputs), id, output)
}
func dbGetSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) (output types.SiacoinOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiacoinOutputs), id, &output)
	return
}
func dbDeleteSiacoinOutput(tx *bolt.Tx, id types.SiacoinOutputID) error {
	return dbDelete(tx.Bucket(bucketSiacoinOutputs), id)
}
func dbForEachSiacoinOutput(tx *bolt.Tx, fn func(types.SiacoinOutputID, types.SiacoinOutput)) error {
	return dbForEach(tx.Bucket(bucketSiacoinOutputs), fn)
}

func dbPutSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID, output types.SiafundOutput) error {
	return dbPut(tx.Bucket(bucketSiafundOutputs), id, output)
}
func dbGetSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) (output types.SiafundOutput, err error) {
	err = dbGet(tx.Bucket(bucketSiafundOutputs), id, &output)
	return
}
func dbDeleteSiafundOutput(tx *bolt.Tx, id types.SiafundOutputID) error {
	return dbDelete(tx.Bucket(bucketSiafundOutputs), id)
}
func dbForEachSiafundOutput(tx *bolt.Tx, fn func(types.SiafundOutputID, types.SiafundOutput)) error {
	return dbForEach(tx.Bucket(bucketSiafundOutputs), fn)
}

func dbPutSpentOutput(tx *bolt.Tx, id types.OutputID, height types.BlockHeight) error {
	return dbPut(tx.Bucket(bucketSpentOutputs), id, height)
}
func dbGetSpentOutput(tx *bolt.Tx, id types.OutputID) (height types.BlockHeight, err error) {
	err = dbGet(tx.Bucket(bucketSpentOutputs), id, &height)
	return
}
func dbDeleteSpentOutput(tx *bolt.Tx, id types.OutputID) error {
	return dbDelete(tx.Bucket(bucketSpentOutputs), id)
}

// bucketProcessedTransactions works a little differently: the key is
// meaningless, only used to order the transactions chronologically.

func dbAppendProcessedTransaction(tx *bolt.Tx, pt modules.ProcessedTransaction) error {
	b := tx.Bucket(bucketProcessedTransactions)
	key, err := b.NextSequence()
	if err != nil {
		return err
	}
	// big-endian is used so that the keys are properly sorted
	keyBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(keyBytes, key)
	return b.Put(keyBytes, encoding.Marshal(pt))
}
func dbGetLastProcessedTransaction(tx *bolt.Tx) (pt modules.ProcessedTransaction, err error) {
	_, val := tx.Bucket(bucketProcessedTransactions).Cursor().Last()
	err = encoding.Unmarshal(val, &pt)
	if err != nil {
		// COMPATv1.2.1: try decoding into old transaction type
		var oldpt v121ProcessedTransaction
		err = encoding.Unmarshal(val, &oldpt)
		pt = convertProcessedTransaction(oldpt)
	}
	return
}
func dbDeleteLastProcessedTransaction(tx *bolt.Tx) error {
	// delete the last entry in the bucket. Note that we don't need to
	// decrement the sequence integer; we only care that the next integer is
	// larger than the previous one.
	b := tx.Bucket(bucketProcessedTransactions)
	key, _ := b.Cursor().Last()
	return b.Delete(key)
}
func dbForEachProcessedTransaction(tx *bolt.Tx, fn func(modules.ProcessedTransaction)) error {
	return dbForEach(tx.Bucket(bucketProcessedTransactions), func(_ uint64, pt modules.ProcessedTransaction) {
		fn(pt)
	})
}

// A processedTransactionsIter iterates through the ProcessedTransactions bucket.
type processedTransactionsIter struct {
	c  *bolt.Cursor
	pt modules.ProcessedTransaction
}

// next decodes the next ProcessedTransaction, returning false if the end of
// the bucket has been reached.
func (it *processedTransactionsIter) next() bool {
	var ptBytes []byte
	if it.pt.TransactionID == (types.TransactionID{}) {
		// this is the first time next has been called, so cursor is not
		// initialized yet
		_, ptBytes = it.c.First()
	} else {
		_, ptBytes = it.c.Next()
	}
	err := encoding.Unmarshal(ptBytes, &it.pt)
	if err != nil {
		// COMPATv1.2.1: try decoding into old transaction type
		var oldpt v121ProcessedTransaction
		err = encoding.Unmarshal(ptBytes, &oldpt)
		it.pt = convertProcessedTransaction(oldpt)
	}
	return err == nil
}

// value returns the most recently decoded ProcessedTransaction.
func (it *processedTransactionsIter) value() modules.ProcessedTransaction {
	return it.pt
}

// dbProcessedTransactionsIterator creates a new processedTransactionsIter.
func dbProcessedTransactionsIterator(tx *bolt.Tx) *processedTransactionsIter {
	return &processedTransactionsIter{
		c: tx.Bucket(bucketProcessedTransactions).Cursor(),
	}
}

// dbGetWalletUID returns the UID assigned to the wallet's primary seed.
func dbGetWalletUID(tx *bolt.Tx) (uid uniqueID) {
	copy(uid[:], tx.Bucket(bucketWallet).Get(keyUID))
	return
}

// dbGetPrimarySeedProgress returns the number of keys generated from the
// primary seed.
func dbGetPrimarySeedProgress(tx *bolt.Tx) (progress uint64, err error) {
	err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyPrimarySeedProgress), &progress)
	return
}

// dbPutPrimarySeedProgress sets the primary seed progress counter.
func dbPutPrimarySeedProgress(tx *bolt.Tx, progress uint64) error {
	return tx.Bucket(bucketWallet).Put(keyPrimarySeedProgress, encoding.Marshal(progress))
}

// dbGetConsensusChangeID returns the ID of the last ConsensusChange processed by the wallet.
func dbGetConsensusChangeID(tx *bolt.Tx) (cc modules.ConsensusChangeID) {
	copy(cc[:], tx.Bucket(bucketWallet).Get(keyConsensusChange))
	return
}

// dbPutConsensusChangeID stores the ID of the last ConsensusChange processed by the wallet.
func dbPutConsensusChangeID(tx *bolt.Tx, cc modules.ConsensusChangeID) error {
	return tx.Bucket(bucketWallet).Put(keyConsensusChange, cc[:])
}

// dbGetConsensusHeight returns the height that the wallet has scanned to.
func dbGetConsensusHeight(tx *bolt.Tx) (height types.BlockHeight, err error) {
	err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keyConsensusHeight), &height)
	return
}

// dbPutConsensusHeight stores the height that the wallet has scanned to.
func dbPutConsensusHeight(tx *bolt.Tx, height types.BlockHeight) error {
	return tx.Bucket(bucketWallet).Put(keyConsensusHeight, encoding.Marshal(height))
}

// dbGetSiafundPool returns the value of the siafund pool.
func dbGetSiafundPool(tx *bolt.Tx) (pool types.Currency, err error) {
	err = encoding.Unmarshal(tx.Bucket(bucketWallet).Get(keySiafundPool), &pool)
	return
}

// dbPutSiafundPool stores the value of the siafund pool.
func dbPutSiafundPool(tx *bolt.Tx, pool types.Currency) error {
	return tx.Bucket(bucketWallet).Put(keySiafundPool, encoding.Marshal(pool))
}

// COMPATv121: these types were stored in the db in v1.2.2 and earlier.
type (
	v121ProcessedInput struct {
		FundType       types.Specifier
		WalletAddress  bool
		RelatedAddress types.UnlockHash
		Value          types.Currency
	}

	v121ProcessedOutput struct {
		FundType       types.Specifier
		MaturityHeight types.BlockHeight
		WalletAddress  bool
		RelatedAddress types.UnlockHash
		Value          types.Currency
	}

	v121ProcessedTransaction struct {
		Transaction           types.Transaction
		TransactionID         types.TransactionID
		ConfirmationHeight    types.BlockHeight
		ConfirmationTimestamp types.Timestamp
		Inputs                []v121ProcessedInput
		Outputs               []v121ProcessedOutput
	}
)

func convertProcessedTransaction(oldpt v121ProcessedTransaction) (pt modules.ProcessedTransaction) {
	pt.Transaction = oldpt.Transaction
	pt.TransactionID = oldpt.TransactionID
	pt.ConfirmationHeight = oldpt.ConfirmationHeight
	pt.ConfirmationTimestamp = oldpt.ConfirmationTimestamp
	pt.Inputs = make([]modules.ProcessedInput, len(oldpt.Inputs))
	for i, in := range oldpt.Inputs {
		pt.Inputs[i] = modules.ProcessedInput{
			FundType:       in.FundType,
			WalletAddress:  in.WalletAddress,
			RelatedAddress: in.RelatedAddress,
			Value:          in.Value,
		}
	}
	pt.Outputs = make([]modules.ProcessedOutput, len(oldpt.Outputs))
	for i, out := range oldpt.Outputs {
		pt.Outputs[i] = modules.ProcessedOutput{
			FundType:       out.FundType,
			MaturityHeight: out.MaturityHeight,
			WalletAddress:  out.WalletAddress,
			RelatedAddress: out.RelatedAddress,
			Value:          out.Value,
		}
	}
	return
}
