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

	var rentStatusInnerHTML = ""
	if(stats.RenterFiles != null) {
		for (s in stats.RenterFiles) {
			if (s != 0) {
				rentStatusInnerHTML += '<br>';
			}
			var formID = 'downloadForm' + s
			rentStatusInnerHTML += '<label>' + stats.RenterFiles[s] + '</label>' +
				'<input type="text" id="' + formID + '" placeholder="destination"></input>' +
				'<button onclick="downloadFile(\'' + stats.RenterFiles[s] + '\', \'' + formID + '\')">Download</button>';
		}
	}
	safeSetElem('rentStatus', rentStatusInnerHTML);

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

	safeSetElem('hostNumContracts', stats.HostContractCount)
	safeSetElem('hostIPAddress', stats.HostSettings.IPAddress.Host + ":" + stats.HostSettings.IPAddress.Port)
	safeSetElem('hostTotalStorage', stats.HostSettings.TotalStorage)
	safeSetElem('hostUnsoldStorage', stats.HostSpaceRemaining)
	safeSetElem('hostMinFilesize', stats.HostSettings.MinFilesize)
	safeSetElem('hostMaxFilesize', stats.HostSettings.MaxFilesize)
	safeSetElem('hostMinDuration', stats.HostSettings.MinDuration)
	safeSetElem('hostMaxDuration', stats.HostSettings.MaxDuration)
	safeSetElem('hostMinWindow', stats.HostSettings.MinChallengeWindow)
	safeSetElem('hostMaxWindow', stats.HostSettings.MaxChallengeWindow)
	safeSetElem('hostMinTolerance', stats.HostSettings.MinTolerance)
	safeSetElem('hostPrice', stats.HostSettings.Price)
	safeSetElem('hostBurn', stats.HostSettings.Burn)
	safeSetElem('hostCoinAddress', stats.HostSettings.CoinAddress)
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
	var nickname = document.getElementById('rentNickname').value;
	responseBoxGet("/rent?sourcefile=" + sourceFile + "&nickname=" + nickname)
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

function downloadFile(nick, destFormID) {
	var dest = document.getElementById(destFormID).value;
	responseBoxGet("/download?nickname="+nick+"&destination="+dest)
}