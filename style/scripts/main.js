var currentView;
$(window).ready(function(){
    $("#money").hide();
    $("#mining").hide();
    $("#manage-account").hide();
    $("#files").hide();
    $("#hosting").hide();
    currentView = "overview";
    $(".accounts .item").click(function(){
        switchView("manage-account");
    });
    $("#back-to-money").click(function(){
        switchView("money");
    });

    // FILE MANAGER
    $("#files .upload-pane").hide();
    $("#files .info-pane").hide();
    $(".file").click(function(e){
        $(".upload-pane").slideUp();
        $(".info-pane").slideDown(function(){
            $(".file-browser").one("click", function(){
                console.log("Browser");
                $(".info-pane").slideUp();
            });
        });
        return false;
    });
    $(".upload-public").click(function(e){
        $(".upload-pane").slideDown();
        $(".info-pane").slideUp();
    });
    $(".upload-pane .stop").click(function(e){
        $(".upload-pane").slideUp();
    });

});

var transitionTypes = {
    "money->manage-account" : "slideleft",
    "manage-account->money" : "slideright"
};

$(window).resize(function(){
    drawMiningGraph();
});

function startLoadingAnimation(){
    $("#loader").css({
        "left": $("#content").width()/2 - $("#loader").width()/2,
        "top": "250px"
    });
    $("#loader").stop().animate({
        "opacity":"1"
    });
    $("#content").children().filter(function(i){
        return this != $("#loader")[0];
    }).fadeOut();
}

function stopLoadingAnimation(newView){
    currentView = newView;
    $("#loader").stop().animate({
        "opacity":"0"
    });
    $("#" + newView).fadeIn();
}

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

function switchView(newView){
    $(".current").removeClass("current");
    $("." + newView + "-button").addClass("current");
    var transitionType = transitionTypes[currentView + "->" + newView] || "load";
    if (transitionType == "load"){
        startLoadingAnimation();
        setTimeout(function(){
            stopLoadingAnimation(newView);
        },400);
    }else if (transitionType == "slideleft"){
        $("#" + newView).show();
        var newElement = $("#" + newView);
        var oldElement = $("#" + currentView);
        newElement.before(oldElement);
        var slideDistance = oldElement.width();
        var heightDifference = oldElement.offset().top - newElement.offset().top;
        newElement.css({"foo":0});
        newElement.animate({foo:1},{
            duration:400,
            step: function(v){
                setTranslate(newElement, slideDistance * (1-v), heightDifference);
                setTranslate(oldElement, slideDistance * -v,0);
            },
            complete:function(){
                clearTransform(oldElement);
                clearTransform(newElement);
                oldElement.hide();
                currentView = newView;
            }
        });
    }else if (transitionType == "slideright"){
        $("#" + newView).show();
        var newElement = $("#" + newView);
        var oldElement = $("#" + currentView);
        newElement.before(oldElement);
        var slideDistance = oldElement.width();
        var heightDifference = oldElement.offset().top - newElement.offset().top;
        newElement.css({"foo":0});
        newElement.animate({foo:1},{
            duration:400,
            step: function(v){
                setTranslate(oldElement, slideDistance * v, 0);
                setTranslate(newElement, slideDistance * (v- 1),heightDifference);
            },
            complete:function(){
                clearTransform(oldElement);
                clearTransform(newElement);
                oldElement.hide();
                currentView = newView;
            }
        });
    }
}

function drawMiningGraph(){
    var parent = $(".graph-container")[0];
    var canvas = $(".graph-container canvas")[0];
    var context = canvas.getContext("2d");

    canvas.width = $(parent).width();
    canvas.height = 200;

    // Draw Grid
    context.lineWidth = 1;
    for (var i = 1;i < 10;i++){
        var xpos = i/10 * canvas.width;
        if (i % 4 === 0){
            context.strokeStyle = "#BDBDBD";
        }else{
            context.strokeStyle = "#EEEEEE";
        }
        context.beginPath();
        context.moveTo(xpos, 0);
        context.lineTo(xpos, canvas.height);
        context.stroke();
        context.closePath();
    }

    context.lineWidth = 1.5;
    context.strokeStyle = "#BDBDBD";
    context.beginPath();
    for (var i = 0;i <= 15;i ++){
        context.lineTo(i/15 * canvas.width, Math.random() * canvas.height);
    }
    context.stroke();
    context.closePath();

}
