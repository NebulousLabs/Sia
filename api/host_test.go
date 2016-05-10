package api

/*
// TestIntegrationRenewing tests that the renter and host manage contract
// renewals properly.
func TestIntegrationRenewing(t *testing.T) {
	st, err := createServerTester("TestIntegrationRenewing")
	if err != nil {
		t.Fatal(err)
	}
	defer st.server.Close()

	// Announce the host.
	err = st.announceHost()
	if err != nil {
		t.Fatal(err)
	}

	// create a file
	path := filepath.Join(build.SiaTestingDir, "api", "TestIntegrationRenewing", "test.dat")
	err = createRandFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// upload to host, specifying that the file should be renewed
	uploadValues := url.Values{}
	uploadValues.Set("source", path)
	err = st.stdPostAPI("/renter/upload/test", uploadValues)
	if err != nil {
		t.Fatal(err)
	}
	// only one piece will be uploaded (10% at current redundancy)
	var rf RenterFiles
	for i := 0; i < 150 && (len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10); i++ {
		st.getAPI("/renter/files", &rf)
		time.Sleep(50 * time.Millisecond)
	}
	if len(rf.Files) != 1 || rf.Files[0].UploadProgress != 10 {
		t.Error(rf.Files[0].UploadProgress)
		t.Fatal("uploading has failed")
	}

	// default expiration is 20 blocks
	expExpiration := st.cs.Height() + 20
	if rf.Files[0].Expiration != expExpiration {
		t.Fatalf("expected expiration of %v, got %v", expExpiration, rf.Files[0].Expiration)
	}

	// mine blocks until we hit the renew threshold (default 10 blocks)
	for st.cs.Height() < expExpiration-10 {
		st.miner.AddBlock()
	}

	// renter should now renew the contract for another 20 blocks
	newExpiration := st.cs.Height() + 20
	for i := 0; i < 5 && rf.Files[0].Expiration != newExpiration; i++ {
		time.Sleep(1 * time.Second)
		st.getAPI("/renter/files", &rf)
	}
}
*/
