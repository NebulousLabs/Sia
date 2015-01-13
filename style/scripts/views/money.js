ui._money = (function(){

    var view, eBalance, eUSDBalance, eAddFunds, eWithdrawFunds, eItems;

    function init(){
        view = $("#money");

        eBalance = view.find(".amt");
        eUSDBalance = view.find(".amtusd");

        eAddFunds = view.find(".add-funds");
        eWithdrawFunds = view.find(".withdraw-funds");

        eItems = $();

        addEvents();
    }

    function addEvents(){

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

    function update(data){
        eBalance.text(data.wallet.Balance);
        if (data.wallet.USDBalance !== undefined){
            eUSDBalance.html("&asymp; " + data.wallet.USDBalance + " USD");
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
            accountElements.push(item[0]);
        });
        eItems = $(accountElements);
        addItemEvents();
    }

    return {
        init:init,
        update:update
    };
})();
