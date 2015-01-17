ui._uploadFile = ui["_upload-file"] = (function(){

    var view, eDescription;

    var privacyType; // "public" or "private"

    function init(){
        view = $("#upload-file");
        eDescription = view.find(".description-field");

        addEvents();
    }

    function addEvents(){
        //TODO: Abstract server call logic to controller
        $(function(){
            $("#fileupload").fileupload({
                datatype: "plaintext",
                add: function(e, data){
                    view.find(".button.upload").off("click").click(function(){
                        data.formData = {
                            "nickname": eDescription.text(),
                            "pieces": 12
                        };
                        data.submit();
                    });
                },
                done: function(e, data){
                    console.log(data._response.result);
                    ui.notify("File successfully uploaded", "sent");
                },
                error: function(err){
                    console.log(arguments);
                    ui.notify("Error uploading: " + err.responseText, "error");
                }
            });
        });

        view.find(".back.button").click(function(){
            ui.switchView("files");
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
