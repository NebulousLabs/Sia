ui._network = (function(){

    var view, eItems, fItems, eItemBlueprint, eAddPeer, eApply;

    function init(){
        view = $("#network");
        eItems = $();
        eItemBlueprint = view.find(".item.blueprint");
        eAddPeer = view.find(".add-peer");
        eApply = view.find(".apply");
    }

    function onViewOpened(data){

        // TODO: remove dummy data here
        if (!data.peers){
            data.peers = {
                "Peers": ["123.456.789:4050","123.456.789:4050","123.456.789:4050"]
            };
        }

        if (data.peers){
            eItems.remove();
            var newEItems = [];
            fItems = [];
            data.peers.Peers.forEach(function(peerAddr){
                console.log(peerAddr);
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
