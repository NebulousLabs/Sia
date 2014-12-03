package siad

import (
	"time"
)

func testHostDatabase(te *testEnv) {
	// Get old number of hosts found in the hostdb.
	oldEntryCount := len(te.e0.hostDatabase.HostList)

	// Check that each wallet has sufficient balance.
	if te.e0.WalletBalance() < 212 {
		te.t.Error("insufficient e0 balance:", te.e0.WalletBalance())
	}
	// Check that each wallet has sufficient balance.
	if te.e1.WalletBalance() < 116 {
		te.t.Error("insufficient e1 balance:", te.e0.WalletBalance())
	}

	// Create transactions which add hosts to the hostdb.
	_, err := te.e0.HostAnnounceSelf(200, te.e0.Height()+1200, 12)
	if err != nil {
		te.t.Error(err)
		return
	}
	_, err = te.e1.HostAnnounceSelf(100, te.e1.Height()+1200, 16)
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
	if len(te.e0.hostDatabase.HostList) != oldEntryCount+2 {
		te.t.Error("number of host entries did not increase, went from", oldEntryCount, "to", len(te.e0.hostDatabase.HostList))
	}
}
