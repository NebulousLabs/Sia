// ui module
// Handles displaying data to UI, user interaction events and changing views,
// basically any interaction with the page should run through UI

// Calling ui.help("<function name>") will give parameter information
// the help will also be displayed if the parameters are given incorrectly

// The UI will get updates to it's data from TODO (external module), update it's
// data object, then calls update(data); on all views.

// Example UI Data Format
/*
{
    "miner":{
        "State":"Off",
        "Threads":1,
        "RunningThreads":0,
        "Address":[126,66,...,173,53]
    },
    "wallet":{
        "Balance":0,
        "USDBalance": 0,
        "FullBalance":0,
        "NumAddresses":2392
    },
    "status": {
        "StateInfo": {
            "CurrentBlock": [231, 83, ..., 114, 111, 159],
            "Height": 0,
            "Target": [0 ,0 , ..., 0 ,0],
            "Depth": [255, 255, ..., 255, 255],
            "EarliestLegalTimestamp": 1417070299
        },
        "RenterFiles": null,
        "IPAddress": ":9988",
        "HostSettings": {
            "IPAddress": ":9988",
            "TotalStorage": 0,
            "MinFilesize": 0,
            "MaxFilesize": 0,
            "MinDuration": 0,
            "MaxDuration": 0,
            "MinChallengeWindow": 0,
            "MaxChallengeWindow": 0,
            "MinTolerance": 0,
            "Price": 0,
            "Burn": 0,
            "CoinAddress": [0 ,0 ,... ,0 ,0]
            "SpendConditions": {
                "TimeLock": 0,
                "NumSignatures": 0,
                "PublicKeys": null
            }
            "FreezeIndex": 0
        },
        "HostSpaceRemaining": 0,
        "HostContractCount": 0
    }
}
*/

var ui = (function(){

    var currentView = "overview";
    var viewNames = ["overview", "money", "manage-account", "files", "hosting", "mining"];
    var transitionTypes = {
        "money->manage-account": "slideleft",
        "manage-account->money": "slideright"
    };
    var lastData;
    var eTooltip;
    var eventListeners = {};

    function switchView(newView){
        // Check that parameter is specified
        if (!newView){
            console.error(help("switchView"));
            return;
        }
        // Check if view is valid
        if (viewNames.indexOf(newView) === -1){
            console.error(newView + " is not a valid view");
            return;
        }

        // Refresh the new view's data
        ui["_" + newView].update(lastData);

        // Make the currently selected button greyed
        $("#sidebar .current").removeClass("current");
        $("." + newView + "-button").addClass("current");

        // Get the animation for the change
        var transitionType = transitionTypes[currentView + "->" + newView] || "load";

        if (transitionType == "load"){
            // Play a dummy loading animation (we may need the time later)
            startLoadingAnimation();
            setTimeout(function(){
                stopLoadingAnimation(newView);
            },400);
        }else if (transitionType == "slideright"){
            slideAnimation(newView, "right");
        }else if (transitionType == "slideleft"){
            slideAnimation(newView, "left");
        }else{
            console.error("Invalid transition type specified");
        }

    }

    function startLoadingAnimation(){
        // Position rotating loader icon in center of content
        $("#loader").css({
            "left": $("#content").width()/2 - $("#loader").width()/2,
            "top": "250px"
        });

        // Animate the loader in
        $("#loader").stop().fadeIn();

        // Make all content (excluding the loader) non-visible
        $("#content").children().filter(function(i){
            return this != $("#loader")[0];
        }).fadeOut();
    }

    function stopLoadingAnimation(newView){
        currentView = newView;
        $("#loader").stop().fadeOut();
        $("#" + newView).fadeIn();
    }

    function slideAnimation(newView, directionString){

        // Utility functions
        function setTranslate(element, x, y){
            element.css({
                "-webkit-transform": "translate(" + x + "px," + y + "px)"
            });
        }

        function clearTransform(element){
            element.css({
                "-webkit-transform": ""
            });
        }


        // Show the new view (off to the side)
        $("#" + newView).show();

        var newElement = $("#" + newView);
        var oldElement = $("#" + currentView);

        // To avoid movement upon removal
        newElement.before(oldElement);

        var slideDistance = oldElement.width();
        var heightDifference = oldElement.offset().top - newElement.offset().top;

        newElement.css({"animationProgress":0});
        newElement.animate({animationProgress:1},{
            duration:400,
            step: function(v){
                if (directionString == "right"){
                    setTranslate(newElement, slideDistance * (v- 1),heightDifference);
                    setTranslate(oldElement, slideDistance * v, 0);
                }else{
                    setTranslate(newElement, slideDistance * (1-v), heightDifference);
                    setTranslate(oldElement, slideDistance * -v,0);
                }
            },
            complete:function(){
                // When the animation is done, clear the transformations and
                // make the current view the primary view
                clearTransform(oldElement);
                clearTransform(newElement);
                oldElement.hide();
                currentView = newView;
            }
        });
    }

    function help(functionName){
        if (!functionName) console.error("help(<function name>)");
        return {
            "switchView": "switchView(<string newView>) \
            \nPossible Views: " + viewNames.join(", "),
            "update": "update(<json data object>) \
            \nData object generated from requests from server, see top of ui.js",
            "addListener": "addListener(<string event>, <function callback>)\
            \nAdd listener when a ui event occurs"
        }[functionName];
    }

    function init(){
        // Hide everything but the "overview" view
        $("#content").children().filter(function(i){
            return this != $("#overview")[0];
        }).hide();

        // Add click listeners to buttons
        viewNames.forEach(function(view){
            $("." + view + "-button").click(function(){
                switchView(view);
            });
        });

        eTooltip = $("#tooltip");

        initViews();
    }

    function initViews(){
        viewNames.forEach(function(view){
            ui["_" + view].init();
        });
    }

    function update(data){
        viewNames.forEach(function(view){
            ui["_" + view].update(data);
        });
        lastData = data;
    }

    // Triggers an event, many ui actions cause triggers
    function _trigger(event){
        console.log("Event Triggered:",event);
        var callbacks = eventListeners[event] || [];
        for (var i = 0;i < callbacks.length;i++){
            callbacks[i]();
        }
    }

    // Shows tooltip with content on given element
    var tooltipTimeout,tooltipVisible;
    function _tooltip(element, content){
        element = $(element);
        eTooltip.show();
        eTooltip.text(content);
        var middleX = element.offset().left + element.width()/2;
        var topY = element.offset().top - element.height();
        eTooltip.offset({
            top: topY - eTooltip.height(),
            left: middleX - eTooltip.width()/2
        });
        if (!tooltipVisible){
            eTooltip.stop();
            eTooltip.css({"opacity":0});
            tooltipVisible = true;
            eTooltip.animate({
                "opacity":1
            },400);
        }else{
            eTooltip.stop();
            eTooltip.show();
            eTooltip.css({"opacity":1});
        }
        clearTimeout(tooltipTimeout);
        tooltipTimeout = setTimeout(function(){
            // eTooltip.hide();
            eTooltip.animate({
                "opacity":"0"
            },400,function(){
                tooltipVisible = false;
                eTooltip.hide();
            });
        },1400);
    }

    function addListener(event, callback){
        eventListeners[event] = eventListeners[event] || [];
        eventListeners[event].push(callback);
    }

    return {
        "switchView": switchView,
        "update": update,
        "addListener": addListener,
        "_tooltip": _tooltip,
        "_trigger": _trigger,
        "help": help,
        "init": init
    };
})();
