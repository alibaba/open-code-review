"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.OmitFromMappedResult = OmitFromMappedResult;
const index_1 = require("../mapped/index");
const omit_1 = require("./omit");
const value_1 = require("../clone/value");
// prettier-ignore
function FromProperties(properties, propertyKeys, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = (0, omit_1.Omit)(properties[K2], propertyKeys, (0, value_1.Clone)(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, propertyKeys, options) {
    return FromProperties(mappedResult.properties, propertyKeys, options);
}
// prettier-ignore
function OmitFromMappedResult(mappedResult, propertyKeys, options) {
    const properties = FromMappedResult(mappedResult, propertyKeys, options);
    return (0, index_1.MappedResult)(properties);
}
