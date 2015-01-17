ui._files = (function(){

    var view, eUploadFile, eFileBlueprint, eFiles;

    function init(){
        view = $("#files");
        eUploadFile = view.find(".upload-public");
        eFileBlueprint = view.find(".file.blueprint");
        eFiles = $();

        addEvents();
    }

    function addEvents(){
        eUploadFile.click(function(e){
            ui._uploadFile.setPrivacy("public");
            ui.switchView("upload-file");
        });
    }

    function onViewOpened(data){
        data.file = {
            "Files":["example.txt","one.txt","two.txt","three.txt","four.txt"]
        };

        if (data.file.Files){
            var files = data.file.Files;
            eFiles.remove();
            var newFileElements = [];
            for (var i = 0;i < files.length;i++){
                var eFile = eFileBlueprint.clone().removeClass("blueprint");
                eFile.find(".name").text(files[i]);
                eFile.find(".size").text("? MB");
                eFileBlueprint.parent().append(eFile);
                newFileElements.push(eFile[0]);
            }
            eFiles = $(newFileElements);
        }
    }

    return {
        init:init,
        onViewOpened: onViewOpened
    };
})();
