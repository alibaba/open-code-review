"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.KeyOfPropertyEntries = KeyOfPropertyEntries;
const indexed_1 = require("../indexed/indexed");
const keyof_property_keys_1 = require("./keyof-property-keys");
/**
 * `[Utility]` Resolves an array of keys and schemas from the given schema. This method is faster
 * than obtaining the keys and resolving each individually via indexing. This method was written
 * accellerate Intersect and Union encoding.
 */
function KeyOfPropertyEntries(schema) {
    const keys = (0, keyof_property_keys_1.KeyOfPropertyKeys)(schema);
    const schemas = (0, indexed_1.IndexFromPropertyKeys)(schema, keys);
    return keys.map((_, index) => [keys[index], schemas[index]]);
}
