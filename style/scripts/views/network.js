ui._network = (function(){

    var view;

    function init(){

    }

    function onViewOpened(data){

        // TODO: remove dummy data here
        if (!data.peers){
            data.peers = {
                "Peers": ["123.456.789:4050","123.456.789:4050","123.456.789:4050"]
            };
        }



    }

    return {
        init: init,
        onViewOpened: onViewOpened
    }

})();
