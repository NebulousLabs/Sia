package main

func testingDaemon() (d *daemon, err error) {
	dc := DaemonConfig{
		APIAddr: ":9020",
		RPCAddr: ":9021",

		HostDir: "hostDir",

		Threads: 2,

		DownloadDir: "downloadDir",

		WalletDir: "walletDir",
	}

	d, err = newDaemon(dc)
	if err != nil {
		return
	}

	go d.listen(dc.APIAddr)
	return
}
