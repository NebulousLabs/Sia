ui._hosting = (function(){

    var view, ePropBlueprint, ePreset16GB, ePreset32GB, ePreset64GB, eProps, eControl;

    var editableProps = ["TotalStorage","MinFilesize","MaxFilesize","MinDuration","MaxDuration","MinChallengeWindow","MaxChallengeWindow","MinTolerance","Price","Burn"];

    function init(){

        view = $("#hosting");

        ePropBlueprint = view.find(".property.blueprint");
        ePreset16GB = view.find(".preset1");
        ePreset32GB = view.find(".preset2");
        ePreset64GB = view.find(".preset3");
        eControl = view.find(".control");
        eProps = $();

    }

    function update(data){
        console.log(data);
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

    }

    return {
        init:init,
        update:update
    };
})();
