ui._network = (function(){

    var view, eItems, fItems, eItemBlueprint, eAddPeer, eApply;

    function init(){
        view = $("#network");
        eItems = $();
        eItemBlueprint = view.find(".item.blueprint");
        eAddPeer = view.find(".add-peer");
        eApply = view.find(".apply");
        fItems = [];

        addEvents();
    }

    function addEvents(){
        eAddPeer.click(function(){
            var eItem = eItemBlueprint.clone().removeClass("blueprint");
            eItemBlueprint.parent().append(eItem);
            eItems = eItems.add(eItem);
            var fItem = ui.FieldElement(eItem.find(".value"));
            fItem.setValue("");
            fItems.push(fItem);
        });
        eApply.click(function(){
            var peerAddresses = fItems.map(function(item){
                return item.getValue();
            });
            ui._trigger("update-peers", peerAddresses);
        });
    }

    function onViewOpened(data){

        if (data.peer){
            eItems.remove();
            var newEItems = [];
            fItems = [];
            data.peer.Peers.forEach(function(peerAddr){
                var eItem = eItemBlueprint.clone().removeClass("blueprint");
                eItemBlueprint.parent().append(eItem);
                newEItems.push(eItem[0]);
                var fItem = ui.FieldElement(eItem.find(".value"));
                fItem.setValue(peerAddr);
                fItems.push(fItem);

            });
            eItems = $(newEItems);
        }



    }

    return {
        init: init,
        onViewOpened: onViewOpened
    };

})();
