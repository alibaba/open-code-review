"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtendsFromMappedResult = ExtendsFromMappedResult;
const index_1 = require("../mapped/index");
const extends_1 = require("./extends");
const value_1 = require("../clone/value");
// prettier-ignore
function FromProperties(P, Right, True, False, options) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = (0, extends_1.Extends)(P[K2], Right, True, False, (0, value_1.Clone)(options));
    return Acc;
}
// prettier-ignore
function FromMappedResult(Left, Right, True, False, options) {
    return FromProperties(Left.properties, Right, True, False, options);
}
// prettier-ignore
function ExtendsFromMappedResult(Left, Right, True, False, options) {
    const P = FromMappedResult(Left, Right, True, False, options);
    return (0, index_1.MappedResult)(P);
}
