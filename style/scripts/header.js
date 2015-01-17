ui._header = (function(){

    var headerElement,eUpdate;

    function init(){
        headerElement = $("#header");
        eUpdate = headerElement.find(".update-button");

        addEvents();
    }

    function addEvents(){
        eUpdate.click(function(){
            controller.promptUserIfUpdateAvailable();
        });
    }

    return {
        init: init
    };

})();
