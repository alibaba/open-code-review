"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.RequiredFromMappedResult = RequiredFromMappedResult;
const index_1 = require("../mapped/index");
const required_1 = require("./required");
// prettier-ignore
function FromProperties(P, options) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = (0, required_1.Required)(P[K2], options);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, options) {
    return FromProperties(R.properties, options);
}
// prettier-ignore
function RequiredFromMappedResult(R, options) {
    const P = FromMappedResult(R, options);
    return (0, index_1.MappedResult)(P);
}
