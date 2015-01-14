var util = (function(){

    var pSI = ["", "&kilo;", "M"];
    var nSI = ["", "m", "&micro;", "&nano;", "&pico;"];

    function engNotation(number, precision){
        precision = precision || 5;
        var sciNotation = number.toExponential();
        var degree = Math.floor(Math.log(Math.abs(number)) / Math.LN10 / 3);

        var numberString = String(number / Math.pow(1000,degree));

        var si = degree > 0 ? pSI[degree] : nSI[degree * -1];

        return numberString.slice(0,precision + 1) + " " + si;
    }

    function USDConvert(balance){
        return balance * 0.00172;
    }

    return {
        "engNotation": engNotation,
        "USDConvert": USDConvert
    };
})();
