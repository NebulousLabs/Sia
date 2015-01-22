package main

func daemonTestConfig() (dc DaemonConfig) {
	return DaemonConfig{
		APIAddr:     ":9020",
		RPCAddr:     ":9021",
		NoBootstrap: true,

		HostDir: "hostDir",

		Threads: 2,

		DownloadDir: "downloadDir",

		WalletDir: "walletDir",
	}
}
