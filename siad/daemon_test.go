package main

func testingDaemon() (d *daemon, err error) {
	dc := DaemonConfig{
		APIAddr:     ":9020",
		RPCAddr:     ":9021",
		NoBootstrap: true,

		HostDir: "hostDir",

		Threads: 2,

		DownloadDir: "downloadDir",

		WalletDir: "walletDir",
	}

	d, err = newDaemon(dc)
	if err != nil {
		return
	}

	go d.handle(dc.APIAddr)
	return
}
