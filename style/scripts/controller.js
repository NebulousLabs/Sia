var controller = (function(){

    var data = {};
    var createdAddressList = [];

    function init(){
        update();
        addListeners();
        setInterval(function(){
            update();
        },250);

        // Wait two seconds then check for a Sia client update
        setTimeout(function(){
            promptUserIfUpdateAvailable();
        },2000);
    }

    function promptUserIfUpdateAvailable(){
        checkForUpdate(function(update){
            if (update.Available){
                ui.notify("New Sia Client Available: Click to update to " + update.Version + "!", "update", function(){
                    updateClient(update.Version);
                });
            }else{
                ui.notify("Sia client up to date!", "success");
            }
        });
    }

    function checkForUpdate(callback){
        $.getJSON("/update/check", callback);
    }

    function updateClient(version){
        $.get("/update/apply", {version:version});
    }

    function httpApiCall(url, params, callback, errorCallback){
        params = params || {};
        $.getJSON(url, params, function(data){
            if (callback) callback(data);
        }).error(function(err){
            if (!errorCallback){
                console.error("BAD CALL TO", url, arguments);
                ui.notify("Error calling " + url + " : " + err.responseText, "error");
            }else{
                errorCallback();
            }
        });
    }

    function addListeners(){
        ui.addListener("add-miner", function(){
            httpApiCall("/miner/start",{
                "threads": data.miner.Threads + 1
            });
        });
        ui.addListener("remove-miner", function(){
            httpApiCall("/miner/start",{
                "threads": data.miner.Threads - 1 < 0 ? 0 : data.miner.Threads - 1
            });
        });
        ui.addListener("toggle-mining", function(){
            if (data.miner.State == "Off"){
                httpApiCall("/miner/start",{
                    "threads": data.miner.Threads
                });
            }else{
                httpApiCall("/miner/stop");
            }
        });
        ui.addListener("stop-mining", function(){
            httpApiCall("/miner/stop");
        });
        ui.addListener("save-host-config", function(hostSettings){
            httpApiCall("/host/setconfig", hostSettings);
        });
        ui.addListener("send-money", function(info){
            ui.wait();
            var address = info.to.address.replace(/[^A-Fa-f0-9]/g, "");
            httpApiCall("/wallet/send", {
                "amount": info.from.amount,
                "dest": address
            }, function(data){
                updateWallet(function(){
                    ui.stopWaiting();
                    ui.switchView("manage-account");
                });
            });
        });
        ui.addListener("create-address", function(){
            ui.wait();
            httpApiCall("/wallet/address",{}, function(info){
                createdAddressList.push({
                    "Address": info.Address,
                    "Balance": 0
                });
                updateWallet(function(){
                    ui.stopWaiting();
                });
            });
        });
        ui.addListener("download-file", function(fileNickname){
            ui.notify("Downloading " + fileNickname + " to Downloads folder", "download");
            httpApiCall("/file/download", {
                "nickname": fileNickname,
                "filename": fileNickname
            });
        });
        ui.addListener("update-peers", function(peers){
            ui.notify("Updating Network...", "peers");

            function addPeer(peerAddr){
                httpApiCall("/peer/add", {
                    "addr": peerAddr
                }, function(){
                    ui.notifySmall("Successfully added peer: " + peerAddr, "success");
                }, function(err){
                    ui.notifySmall("Error adding peer: " + peerAddr, "error");
                });
            }
            function removePeer(peerAddr){
                httpApiCall("/peer/remove", {
                    "addr": peerAddr
                }, function(){
                    ui.notifySmall("Successfully removed peer: " + peerAddr, "success");
                }, function(err){
                    ui.notifySmall("Error removing peer: " + peerAddr, "error");
                });
            }
            var oldPeers = data.peer.Peers;
            for (var i = 0;i < oldPeers.length; i++){
                if (peers.indexOf(oldPeers[i]) == -1){
                    // this peer has been removed
                    removePeer(oldPeers[i]);
                }
            }
            peers.forEach(function(peerAddr){
                if (data.peer.Peers.indexOf(peerAddr) == -1){
                    // This peer needs to be added
                    addPeer(peerAddr);
                }
            });
        });
    }

    var lastUpdateTime = Date.now();
    var lastBalance = 0;
    var runningIncomeRateAverage = 0;

    function updateWallet(callback){
        $.getJSON("/wallet/status", function(response){
            data.wallet = {
                "Balance": response.Balance,
                "FullBalance": response.FullBalance,
                "USDBalance": util.USDConvert(response.Balance),
                "NumAddresses": response.NumAddresses,
                "DefaultAccount": "Main Account",
                "Accounts": [{
                    "Name" : "Main Account",
                    "Balance": response.Balance,
                    "USDBalance": util.USDConvert(response.Balance),
                    "NumAddresses": response.NumAddresses,
                    "Addresses": createdAddressList,
                    "Transactions": []
                }]
            };
            updateUI();
            if (callback) callback();
        });
    }

    function updateMiner(callback){
        $.getJSON("/miner/status", function(response){
            var timeDifference = (Date.now() - lastUpdateTime) * 1000;
            var balance = data.wallet ? data.wallet.Balance : 0;
            var balanceDifference = balance - lastBalance;
            var incomeRate = balanceDifference / timeDifference;
            runningIncomeRateAverage = (runningIncomeRateAverage * 1999 + incomeRate)/2000;
            if (response.State == "Off"){
                runningIncomeRateAverage = 0;
            }
            data.miner = {
                "State": response.State,
                "Threads": response.Threads,
                "RunningThreads": response.RunningThreads,
                "Address": response.Address,
                "AccountName": "Main Account",
                "Balance": balance,
                "USDBalance": util.USDConvert(balance),
                "IncomeRate": runningIncomeRateAverage
            };
            lastBalance = balance;
            lastUpdateTime = Date.now();
            updateUI();
            if (callback) callback();
        });
    }

    function updateStatus(callback){
        $.getJSON("/status", function(response){
            data.status = response;
            updateUI();
            if (callback) callback();
        });
    }

    function updateHost(callback){
        $.getJSON("/host/config", function(response){
            data.host = {
                "HostSettings": response.Announcement
            };
            updateUI();
            if (callback) callback();
        }).error(function(){
            console.log(arguments);
        });
    }

    function updateFile(callback){
        $.getJSON("/file/status", function(response){
            data.file = {
                "Files": response.Files || []
            };
            updateUI();
            if (callback) callback();
        }).error(function(){
            console.log(arguments);
        });
    }

    function updatePeer(callback){
        $.getJSON("/peer/status", function(response){
            data.peer = {
                "Peers": response
            };
            updateUI();
            if (callback) callback();
        }).error(function(){
            console.log(arguments);
        });
    }

    function update(){
        updateWallet();
        updateMiner();
        updateStatus();
        updateHost();
        updateFile();
        updatePeer();
    }

    function updateUI(){
        if (data.wallet && data.miner && data.status && data.host && data.file){
            ui.update(data);
        }
    }

    return {
        "init": init,
        "update": update,
        "promptUserIfUpdateAvailable": promptUserIfUpdateAvailable
    };
})();
