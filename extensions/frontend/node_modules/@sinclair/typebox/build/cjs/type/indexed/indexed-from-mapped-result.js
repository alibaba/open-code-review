"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IndexFromMappedResult = IndexFromMappedResult;
const index_1 = require("../mapped/index");
const indexed_property_keys_1 = require("./indexed-property-keys");
const index_2 = require("./index");
// prettier-ignore
function FromProperties(type, properties, options) {
    const result = {};
    for (const K2 of Object.getOwnPropertyNames(properties)) {
        result[K2] = (0, index_2.Index)(type, (0, indexed_property_keys_1.IndexPropertyKeys)(properties[K2]), options);
    }
    return result;
}
// prettier-ignore
function FromMappedResult(type, mappedResult, options) {
    return FromProperties(type, mappedResult.properties, options);
}
// prettier-ignore
function IndexFromMappedResult(type, mappedResult, options) {
    const properties = FromMappedResult(type, mappedResult, options);
    return (0, index_1.MappedResult)(properties);
}
