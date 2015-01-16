ui._uploadFile = ui["_upload-file"] = (function(){

    var privacyType; // "public" or "private"

    function init(){

    }

    function setPrivacy(_privacyType){
        privacyType = _privacyType;
    }

    function onViewOpened(data){

    }


    return {
        init: init,
        setPrivacy: setPrivacy,
        onViewOpened: onViewOpened
    };
})();
