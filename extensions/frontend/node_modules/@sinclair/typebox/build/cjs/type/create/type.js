"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.CreateType = CreateType;
const policy_1 = require("../../system/policy");
const immutable_1 = require("./immutable");
const value_1 = require("../clone/value");
/** Creates TypeBox schematics using the configured InstanceMode */
function CreateType(schema, options) {
    const result = options !== undefined ? { ...options, ...schema } : schema;
    switch (policy_1.TypeSystemPolicy.InstanceMode) {
        case 'freeze':
            return (0, immutable_1.Immutable)(result);
        case 'clone':
            return (0, value_1.Clone)(result);
        default:
            return result;
    }
}
