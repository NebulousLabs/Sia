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
            }
        });
    }

    function checkForUpdate(callback){
        $.getJSON("/update/check", callback);
    }

    function updateClient(version){
        $.get("/update/apply", {version:version});
    }

    function addListeners(){
        ui.addListener("add-miner", function(){
            $.get("/miner/start",{
                "threads": data.miner.Threads + 1
            }, function(e){
                // TODO: handle error
                console.log(e);
            });
        });
        ui.addListener("remove-miner", function(){
            $.get("/miner/start",{
                "threads": data.miner.Threads - 1 < 0 ? 0 : data.miner.Threads - 1
            }, function(e){
                // TODO: handle error
                console.log(e);
            });
        });
        ui.addListener("toggle-mining", function(){
            if (data.miner.State == "Off"){
                $.get("/miner/start",{
                    "threads": data.miner.Threads
                }, function(e){
                    // TODO: handle error
                    console.log(e);
                });
            }else{
                $.get("/miner/stop", function(e){
                    // TODO: handle error
                    console.log(e);
                });
            }
        });
        ui.addListener("stop-mining", function(){
            $.get("/miner/stop", function(e){
                // TODO: handle error
                console.log(e);
            });
        });
        ui.addListener("save-host-config", function(hostSettings){
            $.get("/host/setconfig", hostSettings, function(e){
                // TODO: handle error
                console.log(e);
            });
        });
        ui.addListener("send-money", function(info){
            ui.wait();
            var address = info.to.address.replace(/[^A-Fa-f0-9]/g, "");
            $.getJSON("/wallet/send", {
                "amount": info.from.amount,
                "dest": address
            }, function(data){
                // TODO: Handle error
                updateWallet(function(){
                    ui.stopWaiting();
                    ui.switchView("manage-account");
                });
            }).error(function(){
                console.log(arguments);
            });
        });
        ui.addListener("create-address", function(){
            ui.wait();
            $.getJSON("/wallet/address", function(info){
                //TODO: Error handling
                console.log(info);
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
            $.getJSON("/file/download", {
                "nickname": fileNickname,
                "filename": fileNickname
            },function(response){
                console.log(response);
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

    function update(){
        updateWallet();
        updateMiner();
        updateStatus();
        updateHost();
        updateFile();
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
