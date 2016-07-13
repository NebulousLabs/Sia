package host

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/types"
)

// TestObligationLocks checks that the storage obligation locking functions
// properly blocks and errors out for various use cases.
func TestObligationLocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestObligationLocks")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Simple lock and unlock.
	ob1 := types.FileContractID{1}
	ht.host.lockStorageObligation(ob1)
	ht.host.unlockStorageObligation(ob1)

	// Simple lock and unlock, with trylock.
	err = ht.host.tryLockStorageObligation(ob1)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	ht.host.unlockStorageObligation(ob1)

	// Threaded lock and unlock.
	blockSuccessful := false
	ht.host.lockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout * 2)
		blockSuccessful = true
		ht.host.unlockStorageObligation(ob1)
	}()
	ht.host.lockStorageObligation(ob1)
	if !blockSuccessful {
		t.Error("two threads were able to simultaneously grab an obligation lock")
	}
	ht.host.unlockStorageObligation(ob1)

	// Attempted lock and unlock - failed.
	ht.host.lockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout * 2)
		ht.host.unlockStorageObligation(ob1)
	}()
	err = ht.host.tryLockStorageObligation(ob1)
	if err != errObligationLocked {
		t.Fatal("storage obligation was able to get a lock, despite already being locked")
	}

	// Attempted lock and unlock - succeeded.
	ht.host.lockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout / 2)
		ht.host.unlockStorageObligation(ob1)
	}()
	err = ht.host.tryLockStorageObligation(ob1)
	if err != nil {
		t.Fatal("storage obligation unable to get lock, depsite having enough time")
	}
	ht.host.unlockStorageObligation(ob1)

	// Multiple locks and unlocks happening together.
	ob2 := types.FileContractID{2}
	ob3 := types.FileContractID{3}
	ht.host.lockStorageObligation(ob1)
	ht.host.lockStorageObligation(ob2)
	ht.host.lockStorageObligation(ob3)
	ht.host.unlockStorageObligation(ob3)
	ht.host.unlockStorageObligation(ob2)
	err = ht.host.tryLockStorageObligation(ob2)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	err = ht.host.tryLockStorageObligation(ob3)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	err = ht.host.tryLockStorageObligation(ob1)
	if err != errObligationLocked {
		t.Fatal("storage obligation was able to get a lock, despite already being locked")
	}
	ht.host.unlockStorageObligation(ob1)
}
