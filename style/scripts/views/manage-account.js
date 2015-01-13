ui._manageAccount = ui["_manage-account"] = (function(){

    var view,eBackToMoney, eBalance, eUSDBalance, eAccountName, eAddFunds, eWithdraw,
        eAddressBlueprint,eAddresses,eTransactionBlueprint,eTransactions;

    var accountName;

    function init(){

        view = $("#manage-account");

        eBalance = view.find(".sumdisplay .amt");
        eUSDBalance = view.find(".sumdisplay .amtusd");
        eAccountName = view.find(".account-name");
        eBackToMoney = $("#back-to-money");
        eAddressBlueprint = view.find(".addresses .item.blueprint");
        eAddresses = $();
        eTransactionBlueprint = view.find(".transactions .item.blueprint");
        eTransactions = $();

        addEvents();
    }

    function addEvents(){
        eBackToMoney.click(function(){
            ui.switchView("money");
        });
    }

    function setAccount(_accountName){
        accountName = _accountName;
        eAccountName.text(accountName);
    }

    function update(data){

        // Find specified account
        var account;
        for (var i = 0;i < data.wallet.Accounts.length;i++){
            if (data.wallet.Accounts[i].Name == accountName){
                account = data.wallet.Accounts[i];
            }
        }

        if (!account){
            console.error("Invalid Account");
            return;
        }

        // TODO this balance should represent the account's balance
        eBalance.text(account.Balance);
        if (account.USDBalance !== undefined){
            eUSDBalance.html("&asymp; " + account.USDBalance + " USD");
        }

        // Populate addresses
        eAddresses.remove();
        eAddresses = $();
        var eItems = [];
        for (var i = 0;i < account.Addresses.length;i++){
            var item = eAddressBlueprint.clone().removeClass("blueprint");
            eAddressBlueprint.parent().append(item);
            item.find(".address").text(account.Addresses[i].Address);
            item.find(".amt").text(account.Addresses[i].Balance);
            eItems.push(item[0]);
        }
        eAddresses = $(eItems);

        // Populate transactions
        eTransactions.remove();
        eTransactions = $();
        eItems = [];
        for (var i = 0;i < account.Transactions.length;i++){
            var item = eTransactionBlueprint.clone().removeClass("blueprint");
            eTransactionBlueprint.parent().append(item);
            item.find(".date").text(account.Transactions[i].Date);
            item.find(".amt").text(account.Transactions[i].Amount);
            var icon = item.find(".icon i");
            icon.removeClass("fa-arrow-right").removeClass("fa-arrow-left");
            if (account.Transactions[i].Deposit){
                icon.addClass("fa-arrow-right");
            }else{
                icon.addClass("fa-arrow-left");
            }
            eItems.push(item[0]);
        }
        eTransactions = $(eItems);
    }

    return {
        init:init,
        setAccount: setAccount,
        update:update
    };
})();
