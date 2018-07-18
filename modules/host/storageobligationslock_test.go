package host

import (
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestObligationLocks checks that the storage obligation locking functions
// properly blocks and errors out for various use cases.
func TestObligationLocks(t *testing.T) {
	if testing.Short() || !build.VLONG {
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
	ht.host.managedLockStorageObligation(ob1)
	ht.host.managedUnlockStorageObligation(ob1)

	// Simple lock and unlock, with trylock.
	err = ht.host.managedTryLockStorageObligation(ob1)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	ht.host.managedUnlockStorageObligation(ob1)

	// Threaded lock and unlock.
	blockSuccessful := false
	ht.host.managedLockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout * 2)
		blockSuccessful = true
		ht.host.managedUnlockStorageObligation(ob1)
	}()
	ht.host.managedLockStorageObligation(ob1)
	if !blockSuccessful {
		t.Error("two threads were able to simultaneously grab an obligation lock")
	}
	ht.host.managedUnlockStorageObligation(ob1)

	// Attempted lock and unlock - failed.
	ht.host.managedLockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout * 2)
		ht.host.managedUnlockStorageObligation(ob1)
	}()
	err = ht.host.managedTryLockStorageObligation(ob1)
	if err != errObligationLocked {
		t.Fatal("storage obligation was able to get a lock, despite already being locked")
	}

	// Attempted lock and unlock - succeeded.
	ht.host.managedLockStorageObligation(ob1)
	go func() {
		time.Sleep(obligationLockTimeout / 2)
		ht.host.managedUnlockStorageObligation(ob1)
	}()
	err = ht.host.managedTryLockStorageObligation(ob1)
	if err != nil {
		t.Fatal("storage obligation unable to get lock, depsite having enough time")
	}
	ht.host.managedUnlockStorageObligation(ob1)

	// Multiple locks and unlocks happening together.
	ob2 := types.FileContractID{2}
	ob3 := types.FileContractID{3}
	ht.host.managedLockStorageObligation(ob1)
	ht.host.managedLockStorageObligation(ob2)
	ht.host.managedLockStorageObligation(ob3)
	ht.host.managedUnlockStorageObligation(ob3)
	ht.host.managedUnlockStorageObligation(ob2)
	err = ht.host.managedTryLockStorageObligation(ob2)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	err = ht.host.managedTryLockStorageObligation(ob3)
	if err != nil {
		t.Fatal("unable to get lock despite not having a lock in place")
	}
	err = ht.host.managedTryLockStorageObligation(ob1)
	if err != errObligationLocked {
		t.Fatal("storage obligation was able to get a lock, despite already being locked")
	}
	ht.host.managedUnlockStorageObligation(ob1)
}
