$(window).ready(function(){
    $(".screenshots img").click(function(){
        var src = $(this).attr("src");
        $(".big-image, .dark-overlay").show();
        $(".big-image").find("img").attr("src", src);
        $(".big-image").animate({
            "opacity": 1
        });
        $(".dark-overlay, .big-image").css({
            "opacity":0
        });
        $(".dark-overlay").animate({
            "opacity":".5"
        });
    });
    $(".big-image, .dark-overlay").click(function(){
        $(".big-image").stop().fadeOut();
        $(".dark-overlay").stop().fadeOut();
    });
    function calcOverlaySize(){
        $(".big-image").css({
            "position": "fixed",
            "opacity": 0,
            "width": window.innerWidth*3/4 + "px",
            "height": window.innerHeight*3/4 + "px",
            "left": window.innerWidth/8  + "px",
            "top": window.innerHeight/8 + "px"
        });
        $(".dark-overlay").css({
            "position":"fixed",
            "left":"0px",
            "top":"0px",
            "opacity": 0,
            "background-color":"#000",
            "width": window.innerWidth,
            "height": window.innerHeight
        });
        $(".big-image, .dark-overlay").hide();
    }
    calcOverlaySize();
    $(window).resize(function(){
        calcOverlaySize();
    });
});
