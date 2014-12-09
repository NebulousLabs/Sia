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

function updatePage() {
	var resp = httpGet("/json/status");
	var stats = JSON.parse(resp);

	safeSetValue('hostCoinAddress', stats.WalletAddress);

	var rentStatusInnerHTML
	// alert(stats.Files)
	// for (String s : stats.Files) {
		// rentStatusInnerHTML = rentStatusInnerHTML + s + '<br>';
	// }

	safeSetElem('miningStatus', 'Mining Status: ' + stats.Mining);
	safeSetElem('blockStatus',
		'Current Height: ' + stats.StateInfo.Height +
		'<br>Current Target: ' + stats.StateInfo.Target +
		'<br>Current Depth: ' + stats.StateInfo.Depth
	);
	safeSetElem('walletStatus', 'Wallet Balance: ' + stats.WalletBalance + '<br>Wallet Address: ' + stats.WalletAddress);
	safeSetElem('hostStatus',
		'In progess'
	);
	safeSetElem('hostStatus',
		'Host Total Storage: ' + stats.HostSettings.TotalStorage +
		'<br>Host Unsold Storage: ' + stats.HostSpaceRemaining +
		'<br>Host Number of Contracts: ' + stats.HostContractCount
	);
	safeSetElem('hostFullStatus',
		'IP Address: ' + stats.HostSettings.IPAddress.Host + ":" + stats.HostSettings.IPAddress.Port +
		'<br>Total Storage: ' + stats.HostSettings.TotalStorage +
		'<br>Unsold Storage: ' + stats.HostSpaceRemaining +
		'<br>Number of Contracts: ' + stats.HostContractCount +
		'<br>Min Filesize: ' + stats.HostSettings.MinFilesize +
		'<br>Max Filesize: ' + stats.HostSettings.MaxFilesize +
		'<br>Min Duration: ' + stats.HostSettings.MinDuration +
		'<br>Max Duration: ' + stats.HostSettings.MaxDuration +
		'<br>Min Challenge Window: ' + stats.HostSettings.MinChallengeWindow +
		'<br>Max Challenge Window: ' + stats.HostSettings.MaxChallengeWindow +
		'<br>Min Tolerance: ' + stats.HostSettings.MinTolerance +
		'<br>Price: ' + stats.HostSettings.Price +
		'<br>Burn: ' + stats.HostSettings.Burn +
		'<br>CoinAddress: ' + stats.HostSettings.CoinAddress
	);
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

function sendMoney() {
	var destination = document.getElementById('destinationAddress').value;
	var amount = document.getElementById('amountToSend').value;
	var fee = document.getElementById('minerFee').value;
	var request = "/sendcoins?amount="+amount+"&fee="+fee+"&dest="+destination;
	responseBoxGet(request);
}

function rentFile() {
	var sourceFile = document.getElementById('rentSourceFile').value;
	responseBoxGet("/rent?sourcefile=" + sourceFile)
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
