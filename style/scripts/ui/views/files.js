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
        if (data.file.Files){
            var files = data.file.Files;
            eFiles.remove();
            var newFileElements = [];
            files.forEach(function(fileNickname){
                var eFile = eFileBlueprint.clone().removeClass("blueprint");
                eFile.find(".name").text(fileNickname);
                eFile.find(".size").text("? MB");
                eFileBlueprint.parent().append(eFile);
                newFileElements.push(eFile[0]);
                eFile.click(function(){
                    ui._trigger("download-file", fileNickname);
                });
            });
            eFiles = $(newFileElements);
        }
    }

    return {
        init:init,
        onViewOpened: onViewOpened
    };
})();
