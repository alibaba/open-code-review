"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.PartialFromMappedResult = PartialFromMappedResult;
const index_1 = require("../mapped/index");
const partial_1 = require("./partial");
const value_1 = require("../clone/value");
// prettier-ignore
function FromProperties(K, options) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(K))
        Acc[K2] = (0, partial_1.Partial)(K[K2], (0, value_1.Clone)(options));
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, options) {
    return FromProperties(R.properties, options);
}
// prettier-ignore
function PartialFromMappedResult(R, options) {
    const P = FromMappedResult(R, options);
    return (0, index_1.MappedResult)(P);
}
