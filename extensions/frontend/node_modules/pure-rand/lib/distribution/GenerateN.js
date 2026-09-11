"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.generateN = generateN;
var UnsafeGenerateN_1 = require("./UnsafeGenerateN");
function generateN(rng, num) {
    var nextRng = rng.clone();
    var out = (0, UnsafeGenerateN_1.unsafeGenerateN)(nextRng, num);
    return [out, nextRng];
}
