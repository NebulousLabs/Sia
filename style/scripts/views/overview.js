// This module is for interaction with the overview view

ui._overview = (function(){

    var view, eMiningStatus, eHostingStatus, eBalance, eUSDBalance, eAddFunds, eWithdrawFunds;

    function init(){
        view = $("#overview");

        // Dashboard first header
        eMiningStatus = view.find(".mining-status");
        eHostingStatus = view.find(".hosting-status");

        // Dashboard second header
        eBalance = view.find(".amt");
        eUSDBalance = view.find(".amtusd");

        eAddFunds = view.find(".add-funds");
        eWithdrawFunds = view.find(".withdraw-funds");

        addEvents();
    }

    function addEvents(){

        eMiningStatus.click(function(){
            ui._trigger("toggle-mining");
        });
        eHostingStatus.click(function(){
            ui._trigger("toggle-hosting");
        });

        eAddFunds.click(function(){
            console.error("NOT IMPLEMENTED");
            // TODO enable exchange view
            // ui._exchange.setTarget("all");
            // ui._exchange.setSource(null);
            // ui.switchView("exchange");
        });
        eWithdrawFunds.click(function(){
            console.error("NOT IMPLEMENTED");
            // TODO enable exchange view
            // ui._exchange.setTarget(null);
            // ui._exchange.setSource("all");
            // ui.switchView("exchange");
        });
    }

    function update(data){
        // First Header
        if (data.miner.State == "Off"){
            eMiningStatus.text("Miner Off");
            eMiningStatus.removeClass("enabled");
            eMiningStatus.addClass("disabled");
        }else{
            eMiningStatus.text("Miner On");
            eMiningStatus.removeClass("disabled");
            eMiningStatus.addClass("enabled");
        }

        // Second Header
        eBalance.text(data.wallet.Balance);
        if (data.wallet.USDBalance !== undefined){
            eUSDBalance.html("&asymp; " + data.wallet.USDBalance + " USD");
        }
    }

    return {
        "init": init,
        "update": update
    };

})();
