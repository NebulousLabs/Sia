ui._transferFunds = ui["_transfer-funds"] = (function(){

    var view, fAccountTransfer, fAccountFinalBalance, eAccountBalance, fAddressAddress;


    // Field elements have a pencil icon, when they're editted the pencil
    // disappears
    function FieldElement(element, onChange){

        var icon = element.find(".icon");

        element.click(function(){
            icon.remove();
            element.attr("contentEditable","true");
            element.focus();
        });

        element.keydown(function(e){
            if (e.keyCode == 13){
                element.blur();
            }
        })

        element.blur(function(){
            element.attr("contentEditable","");
            if (onChange) onChange(element.text());
            element.prepend(icon);
        });

        return {
            element: element,
            onChange: onChange
        };
    }

    function init(){
        view = $("#transfer-funds");
        fAccountTransfer = FieldElement(view.find(".account .field-transfer"));
        fAccountFinalBalance = FieldElement(view.find(".account .field-final-balance"));
        fAddressAddress = FieldElement(view.find(".address .field-address"));
    }

    function addEvents(){
        fAccountTransfer.onChange = function(){

        };
        fAccountFinalBalance.onChange = function(){

        };
        fAddressAddress.onChange = function(){

        };
    }

    function update(data){

    }

    // Sets where funds should be coming out of
    function setFrom(type, info){

    }

    // Sets where funds should be going to
    function setTo(type, info){

    }

    return {
        init:init,
        update:update,
        setFrom: setFrom,
        setTo: setTo
    };
})();
