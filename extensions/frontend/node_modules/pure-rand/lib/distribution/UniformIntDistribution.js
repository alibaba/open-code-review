"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.uniformIntDistribution = uniformIntDistribution;
var UnsafeUniformIntDistribution_1 = require("./UnsafeUniformIntDistribution");
function uniformIntDistribution(from, to, rng) {
    if (rng != null) {
        var nextRng = rng.clone();
        return [(0, UnsafeUniformIntDistribution_1.unsafeUniformIntDistribution)(from, to, nextRng), nextRng];
    }
    return function (rng) {
        var nextRng = rng.clone();
        return [(0, UnsafeUniformIntDistribution_1.unsafeUniformIntDistribution)(from, to, nextRng), nextRng];
    };
}
