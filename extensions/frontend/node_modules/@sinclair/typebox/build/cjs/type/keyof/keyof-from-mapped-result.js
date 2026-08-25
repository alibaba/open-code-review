"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.KeyOfFromMappedResult = KeyOfFromMappedResult;
const index_1 = require("../mapped/index");
const keyof_1 = require("./keyof");
const value_1 = require("../clone/value");
// prettier-ignore
function FromProperties(properties, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = (0, keyof_1.KeyOf)(properties[K2], (0, value_1.Clone)(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, options) {
    return FromProperties(mappedResult.properties, options);
}
// prettier-ignore
function KeyOfFromMappedResult(mappedResult, options) {
    const properties = FromMappedResult(mappedResult, options);
    return (0, index_1.MappedResult)(properties);
}
