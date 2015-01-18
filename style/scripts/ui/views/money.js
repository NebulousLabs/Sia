ui._money = (function(){

    var view, eBalance, eUSDBalance, eAddFunds, eWithdrawFunds, eItems, eAddAccount,
        eTransferFunds, eDeleteAccount;

    function init(){
        view = $("#money");

        eBalance = view.find(".amt");
        eUSDBalance = view.find(".amtusd");

        eAddFunds = view.find(".add-funds");
        eWithdrawFunds = view.find(".withdraw");

        eAddAccount = view.find(".add-account");
        eTransferFunds = view.find(".transfer-funds");
        eDeleteAccount = view.find(".delete-account");

        eItems = $();

        addEvents();
    }

    function addEvents(){
        eAddFunds.click(function(){
            ui._tooltip(this, "Not Implemented");
        });
        eWithdrawFunds.click(function(){
            ui._tooltip(this, "Not Implemented");
        });
        eAddAccount.click(function(){
            ui._tooltip(this, "Not Implemented");
        });
        eTransferFunds.click(function(){
            ui._tooltip(this, "Not Implemented");
        });
        eDeleteAccount.click(function(){
            ui._tooltip(this, "Not Implemented");
        });
    }

    function addItemEvents(){
        eItems.each(function(){
            var item = $(this);
            item.click(function(){
                var accountName = item.find(".name")[0].innerHTML;
                ui._manageAccount.setAccount(accountName);
                ui.switchView("manage-account");
            });
        });
    }

    function onViewOpened(data){
        eBalance.html(util.engNotation(data.wallet.Balance) + "SC");
        if (data.wallet.USDBalance !== undefined){
            eUSDBalance.html("&asymp; " + util.engNotation(data.wallet.USDBalance) + "USD");
        }

        eItems.remove();
        eItems = $();

        // Load account elements
        var blueprint = $(".accounts .item.blueprint");
        var accountElements = [];
        data.wallet.Accounts.forEach(function(account){
            var item = blueprint.clone().removeClass("blueprint");
            blueprint.parent().append(item);
            item.find(".name").text(account.Name);
            item.find(".money").text(util.engNotation(account.Balance,10) + "SC");
            item.find(".money").hover(function(){
                ui._tooltip(this, util.engNotation(account.USDBalance) + "USD");
            });
            accountElements.push(item[0]);
        });
        eItems = $(accountElements);
        addItemEvents();
    }

    return {
        init:init,
        onViewOpened:onViewOpened
    };
})();
