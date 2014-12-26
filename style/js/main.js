function safeSetElem(field, value) {
	var elem = document.getElementById(field)
	if (elem != null) {
		elem.innerHTML = value
	}
}

function safeSetValue(field, value) {
	var elem = document.getElementById(field)
	if (elem != null) {
		elem.defaultValue = value
	}
}

function continuousUpdate() {
	var resp = httpGet("/json/status");
	var stats = JSON.parse(resp);

	var miningResp = httpGet("/miner/status")
	var miningStats = JSON.parse(miningResp)
	safeSetElem('miningStatus',
		'Mining Status: ' + miningStats.State +
		'<br>Threads: ' + miningStats.RunningThreads
	);

	safeSetElem('blockStatus',
		'Current Height: ' + stats.StateInfo.Height +
		'<br>Current Target: ' + stats.StateInfo.Target +
		'<br>Current Depth: ' + stats.StateInfo.Depth
	);
	safeSetElem('hostStatus',
		'Host Total Storage: ' + stats.HostSettings.TotalStorage +
		'<br>Host Unsold Storage: ' + stats.HostSpaceRemaining +
		'<br>Host Number of Contracts: ' + stats.HostContractCount
	);

	var walletResp = httpGet("/wallet/status")
	var walletStats = JSON.parse(walletResp)
	safeSetElem('walletStatus', 
		'Unconfirmed Balance: ' + walletStats.Balance +
		'<br>Full Balance: ' + walletStats.FullBalance +
		'<br>Number of Addresses: ' + walletStats.NumAddresses
	);

	return stats
}

function updatePage() {
	var stats = continuousUpdate()
	safeSetValue('hostCoinAddress', stats.WalletAddress);

	/*
	var rentStatusInnerHTML = ""
	if(stats.RenterFiles != null) {
		for (s in stats.RenterFiles) {
			rentStatusInnerHTML += '<label>' + stats.RenterFiles[s] + '</label>' +
				'<button onclick="downloadFile(\'' + stats.RenterFiles[s] + '\')">Download</button><br>';
		}
	}
	safeSetElem('rentStatus', rentStatusInnerHTML);

	safeSetElem('hostNumContracts', stats.HostContractCount)
	if (stats.HostSettings.TotalStorage > 0) {
		safeSetElem('hostAcceptingContracts', "are")
	} else {
		safeSetElem('hostAcceptingContracts', "are not")
	}

	safeSetValue('hostIPAddress', stats.HostSettings.IPAddress)
	// safeSetValue('hostTotalStorage', stats.HostSettings.TotalStorage)
	// safeSetValue('hostUnsoldStorage', stats.HostSpaceRemaining)
	// safeSetValue('hostMinFilesize', stats.HostSettings.MinFilesize)
	// safeSetValue('hostMaxFilesize', stats.HostSettings.MaxFilesize)
	// safeSetValue('hostMinDuration', stats.HostSettings.MinDuration)
	// safeSetValue('hostMaxDuration', stats.HostSettings.MaxDuration)
	// safeSetValue('hostMinWindow', stats.HostSettings.MinChallengeWindow)
	// safeSetValue('hostMaxWindow', stats.HostSettings.MaxChallengeWindow)
	// safeSetValue('hostMinTolerance', stats.HostSettings.MinTolerance)
	// safeSetValue('hostPrice', stats.HostSettings.Price)
	// safeSetValue('hostBurn', stats.HostSettings.Burn)
	// safeSetValue('hostCoinAddress', stats.HostSettings.CoinAddress)
	*/
}

function httpGet(url) {
	var xmlHttp = null;
	xmlHttp = new XMLHttpRequest();
	xmlHttp.open("GET", url, false);
	xmlHttp.send(null);
	return xmlHttp.responseText;
}

function responseBoxGet(url) {
	safeSetElem('apiResponse', httpGet(url));
	updatePage()
}

function turnOnMiner() {
	var threads = document.getElementById('minerThreads').value;
	var request = "/miner/start?threads="+threads;
	responseBoxGet(request);
}

function turnOffMiner() {
	responseBoxGet("/miner/stop");
}

function reqAddress() {
	responseBoxGet("/wallet/address")
}

function sendMoney() {
	var destination = document.getElementById('destinationAddress').value;
	var amount = document.getElementById('amountToSend').value;
	var request = "/wallet/send?amount="+amount+"&dest="+destination;
	responseBoxGet(request);
}

/*
function rentFile() {
	var sourceFile = document.getElementById('rentSourceFile').value.replace('file://', '');
	var nickname = document.getElementById('rentNickname').value;
	if (nickname != "") {
		responseBoxGet("/rent?sourcefile=" + sourceFile + "&nickname=" + nickname)
	} else {
		safeSetElem('apiResponse', "Please provide a nickname");
	}
}

function hostAnnounce() {
	var address = document.getElementById('hostIPAddress').value;
	var totalstorage = document.getElementById('hostTotalStorage').value;
	var minfile = document.getElementById('hostMinFilesize').value;
	var maxfile = document.getElementById('hostMaxFilesize').value;
	var minduration = document.getElementById('hostMinDuration').value;
	var maxduration = document.getElementById('hostMaxDuration').value;
	var minwin = document.getElementById('hostMinWindow').value;
	var maxwin = document.getElementById('hostMaxWindow').value;
	var mintolerance = document.getElementById('hostMinTolerance').value;
	var price = document.getElementById('hostPrice').value;
	var penalty = document.getElementById('hostBurn').value;
	var coinaddress = document.getElementById('hostCoinAddress').value;
	var freezevolume = document.getElementById('hostFreezeVolume').value;
	var freezeduration = document.getElementById('hostFreezeDuration').value;

	var request = "/host?ipaddress="+address+
		"&totalstorage="+totalstorage+
		"&minfile="+minfile+
		"&maxfile="+maxfile+
		"&minduration="+minduration+
		"&maxduration="+maxduration+
		"&minwin="+minwin+
		"&maxwin="+maxwin+
		"&mintolerance="+mintolerance+
		"&price="+price+
		"&penalty="+penalty+
		"&coinaddress="+coinaddress+
		"&freezevolume="+freezevolume+
		"&freezeduration="+freezeduration;

	responseBoxGet(request);
}

function downloadFile(nick) {
	responseBoxGet("/download?nickname="+nick);
}
*/
