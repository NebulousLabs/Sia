ui._mining = (function(){

    var view, eMiningStatus, eIncomeRate, eActiveMiners, eActiveMinerCount, eAddMiner,
        eRemoveMiner, eStopMining, eAccountName, eBalance, eUSDBalance;

    function init(){
        view = $("#mining");
        eMiningStatus = view.find(".mining-status");
        eIncomeRate = view.find(".income-rate");
        eActiveMiners = view.find(".active-miners");
        eActiveMinerCount = view.find(".miner-control .display .number");
        eAddMiner = view.find(".add-miner");
        eRemoveMiner = view.find(".remove-miner");
        eStopMining = view.find(".stop-mining");
        eAccountName = view.find(".account-name");
        eBalance = view.find(".account-info .amt");
        eUSDBalance = view.find(".account-info .amtusd");

        addEvents();
    }

    function addEvents(){
        eMiningStatus.click(function(){
            ui._tooltip(eMiningStatus, "Not Implemented");
            ui._trigger("toggle-mining");
        });
        eAddMiner.click(function(){
            ui._tooltip(this, "Not Implemented");
            ui._trigger("add-miner");
        });
        eRemoveMiner.click(function(){
            ui._tooltip(this, "Not Implemented");
            ui._trigger("remove-miner");
        });
        eStopMining.click(function(){
            ui._tooltip(this, "Not Implemented");
            ui._trigger("stop-mining");
        });
    }

    function update(data){
        var minerOn = data.miner.State == "Off" ? false : true;
        if (data.miner.Threads < 1){
            minerOn = false;
        }

        if (!minerOn){
            eMiningStatus.text("Mining Off");
            eMiningStatus.removeClass("enabled");
            eMiningStatus.addClass("disabled");
            eActiveMiners.text("No Active Threads");
        }else{
            eMiningStatus.text("Mining On");
            eMiningStatus.removeClass("disabled");
            eMiningStatus.addClass("enabled");
            eActiveMiners.text(data.miner.RunningThreads + " Active Threads");
        }

        eActiveMinerCount.text(data.miner.RunningThreads);
        eIncomeRate.text(data.miner.IncomeRate);

        eBalance.text(data.miner.Balance);
        if (data.miner.USDBalance !== undefined){
            eUSDBalance.html("&asymp; " + data.wallet.USDBalance + " USD");
        }
    }

    return {
        init:init,
        update:update
    };
})();
