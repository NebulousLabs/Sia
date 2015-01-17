// Field elements have a pencil icon, when they're editted the pencil
// disappears
ui.FieldElement = function(element, changeListener){

    var icon = element.find(".icon");

    var onChangeListener = changeListener || function(){};

    element.click(function(){
        icon.remove();
        element.attr("contentEditable","true");
        element.focus();
    });

    element.keydown(function(e){
        if (e.keyCode == 13){
            element.blur();
        }
    });

    element.blur(function(){
        element.attr("contentEditable","");
        onChangeListener(element.text());
        element.prepend(icon);
    });

    function setValue(text){
        icon.remove();
        element.text(text || " ");
        element.prepend(icon);
    }

    function getValue(){
        icon.remove();
        var text = element.text();
        element.prepend(icon);
        return text;
    }

    function setOnChangeListener(callback){
        onChangeListener = callback;
    }

    function triggerChange(){
        onChangeListener(getValue());
    }

    return {
        element: element,
        setOnChangeListener: setOnChangeListener,
        triggerChange: triggerChange,
        setValue: setValue,
        getValue: getValue
    };
};
