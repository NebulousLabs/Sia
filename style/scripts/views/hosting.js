ui._hosting = (function(){

    var view, ePropBlueprint, ePreset16GB, ePreset32GB, ePreset64GB, eProps, eControl, eSave, eReset;

    var editableProps = ["TotalStorage","MinFilesize","MaxFilesize","MinDuration","MaxDuration","MinChallengeWindow","MaxChallengeWindow","MinTolerance","Price","Burn"];

    var lastHostSettings;

    function init(){

        view = $("#hosting");

        ePropBlueprint = view.find(".property.blueprint");
        ePreset16GB = view.find(".preset1");
        ePreset32GB = view.find(".preset2");
        ePreset64GB = view.find(".preset3");
        eControl = view.find(".control");
        eProps = $();
        eSave = view.find(".control .save");
        eReset = view.find(".control .reset");

        addEvents();
    }

    function addEvents(){
        eSave.click(function(){
            ui._tooltip(this, "Saving");
            ui._trigger("save-host-config", parseHostSettings());
        });
        eReset.click(function(){
            ui._tooltip(this, "Reseting");
            for (var i = 0;i < editableProps.length;i ++){
                var item = $(eProps[i]);
                // item.find(".value").text(lastHostSettings[editableProps[i]]);
            }
        });
    }

    function parseHostSettings(){
        var newSettings = {};
        for (var i = 0;i < editableProps.length;i ++){
            var item = $(eProps[i]);
            newSettings[editableProps[i]] = item.find(".value").text();
        }
        return newSettings;
    }

    function update(data){
        // If this is the first time, create and load all properties
        if (eProps.length === 0){
            for (var i = 0; i < editableProps.length; i++){
                var item = ePropBlueprint.clone().removeClass("blueprint");
                ePropBlueprint.parent().append(item);
                eProps.add(item);
                item.find(".name").text(editableProps[i]);
                view.append(eControl);
                // item.find(".value").text(data.hosting.HostingSettings[editableProps[i]]);
            }
        }

        // lastHostSettings = data.hosting.HostingSettings;

    }

    return {
        init:init,
        update:update
    };
})();
