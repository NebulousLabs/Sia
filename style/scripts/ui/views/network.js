ui._network = (function(){


    function init(){
        view = $("#network");
        eItems = $();
        eItemBlueprint = view.find(".item.blueprint");
        eAddPeer = view.find(".add-peer");
        eApply = view.find(".apply");

        addEvents();
    }

    function addEvents(){
        eAddPeer.click(function(){
            addPeer("");
        });
        eApply.click(function(){
            applyChanges();
        });
    }

    function applyChanges(){
        var peerAddresses = $.map(eItems, function(item,i){
            return $(item).find(".value").val();
        });
        ui._trigger("update-peers", peerAddresses);
    }

    function addPeer(peerAddr){
        var eItem = eItemBlueprint.clone().removeClass("blueprint");
        eItemBlueprint.parent().append(eItem);
        eItem.find(".cancel").click(function(){
            eItem.remove();
        });
        eItem.find(".value").change(function(){
            applyChanges();
        });
        eItem.find(".value").val(peerAddr);
        eItems = eItems.add(eItem);
    }

    function onViewOpened(data){

        if (data.peer){
            eItems.remove();
            eItems = $();
            data.peer.Peers.forEach(function(peerAddr){
                addPeer(peerAddr);
            });
        }

    }

    return {
        init: init,
        onViewOpened: onViewOpened
    };

})();
