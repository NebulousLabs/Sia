ui._files = (function(){

    var view, eUploadFile;

    function init(){
        view = $("#files");
        eUploadFile = view.find(".upload-public");

        addEvents();
    }

    function addEvents(){
        eUploadFile.click(function(e){
            ui._uploadFile.setPrivacy("public");
            ui.switchView("upload-file");
        });
    }

    function update(){
    }

    return {
        init:init,
        update:update
    };
})();
