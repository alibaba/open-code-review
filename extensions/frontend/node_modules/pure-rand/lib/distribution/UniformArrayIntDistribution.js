"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.uniformArrayIntDistribution = uniformArrayIntDistribution;
var UnsafeUniformArrayIntDistribution_1 = require("./UnsafeUniformArrayIntDistribution");
function uniformArrayIntDistribution(from, to, rng) {
    if (rng != null) {
        var nextRng = rng.clone();
        return [(0, UnsafeUniformArrayIntDistribution_1.unsafeUniformArrayIntDistribution)(from, to, nextRng), nextRng];
    }
    return function (rng) {
        var nextRng = rng.clone();
        return [(0, UnsafeUniformArrayIntDistribution_1.unsafeUniformArrayIntDistribution)(from, to, nextRng), nextRng];
    };
}
