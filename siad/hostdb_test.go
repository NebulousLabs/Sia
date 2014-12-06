package main

import (
	"time"
)

func testHostDatabase(te *testEnv) {
	// Get old number of hosts found in the hostdb.
	e0OldEntryCount := len(te.e0.hostDatabase.HostList)
	e1OldEntryCount := len(te.e1.hostDatabase.HostList)

	// Check that each wallet has sufficient balance.
	if te.e0.WalletBalance() < 112 {
		te.t.Error("insufficient e0 balance:", te.e0.WalletBalance())
	}
	// Check that each wallet has sufficient balance.
	if te.e1.WalletBalance() < 216 {
		te.t.Error("insufficient e1 balance:", te.e0.WalletBalance())
	}

	// Create transactions which add hosts to the hostdb.
	_, err := te.e0.HostAnnounceSelf(100, te.e0.Height()+1200, 12)
	if err != nil {
		te.t.Error(err)
		return
	}
	_, err = te.e1.HostAnnounceSelf(200, te.e1.Height()+1200, 16)
	if err != nil {
		te.t.Error(err)
		return
	}
	time.Sleep(300 * time.Millisecond)

	// Mine a transaction to get the host announcements through, and then give
	// the block time to propagate.
	te.e0.mineSingleBlock()
	time.Sleep(300 * time.Millisecond)

	// Check that hostdb has new entries.
	te.e0.hostDatabase.RLock()
	te.e1.hostDatabase.RLock()
	if len(te.e0.hostDatabase.HostList) != e0OldEntryCount+2 {
		te.t.Error("number of host entries did not increase, went from", e0OldEntryCount, "to", len(te.e0.hostDatabase.HostList))
	}
	// Check that hostdb has new entries.
	if len(te.e1.hostDatabase.HostList) != e1OldEntryCount+2 {
		te.t.Error("number of host entries did not increase, went from", e1OldEntryCount, "to", len(te.e1.hostDatabase.HostList))
	}
	te.e0.hostDatabase.RUnlock()
	te.e1.hostDatabase.RUnlock()

	// Check that ChooseHost() favors e0.
	var e0Picks int
	var e1Picks int
	for i := 0; i < 100; i++ {
		te.e0.hostDatabase.RLock()
		host, err := te.e0.hostDatabase.ChooseHost()
		te.e0.hostDatabase.RUnlock()
		if err != nil {
			te.t.Error(err)
			return
		}
		if host.IPAddress.Port == 9988 {
			e0Picks++
		} else if host.IPAddress.Port == 9989 {
			e1Picks++
		} else {
			te.t.Fatal(host)
		}
	}

	if e0Picks >= e1Picks {
		te.t.Error("e0Picks is not being favored!")
	}
}
