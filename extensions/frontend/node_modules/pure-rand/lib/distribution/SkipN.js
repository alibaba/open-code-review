"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.skipN = skipN;
var UnsafeSkipN_1 = require("./UnsafeSkipN");
function skipN(rng, num) {
    var nextRng = rng.clone();
    (0, UnsafeSkipN_1.unsafeSkipN)(nextRng, num);
    return nextRng;
}
