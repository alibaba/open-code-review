"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.PickFromMappedResult = PickFromMappedResult;
const index_1 = require("../mapped/index");
const pick_1 = require("./pick");
const value_1 = require("../clone/value");
// prettier-ignore
function FromProperties(properties, propertyKeys, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = (0, pick_1.Pick)(properties[K2], propertyKeys, (0, value_1.Clone)(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, propertyKeys, options) {
    return FromProperties(mappedResult.properties, propertyKeys, options);
}
// prettier-ignore
function PickFromMappedResult(mappedResult, propertyKeys, options) {
    const properties = FromMappedResult(mappedResult, propertyKeys, options);
    return (0, index_1.MappedResult)(properties);
}
