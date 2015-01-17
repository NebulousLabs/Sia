ui._uploadFile = ui["_upload-file"] = (function(){

    var view;

    var privacyType; // "public" or "private"

    function init(){
        view = $("#upload-file");
        $(function(){
            $("#fileupload").fileupload({
                datatype: "plaintext",
                add: function(e, data){
                    console.log("File Added");
                    view.find(".button.upload").off("click").click(function(){
                        console.log("Attempting upload");
                        data.submit();
                    });
                },
                done: function(e, data){
                    console.log("Upload Done");
                }
            });
        });
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
