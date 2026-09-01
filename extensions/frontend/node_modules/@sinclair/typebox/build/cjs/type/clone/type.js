"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.CloneRest = CloneRest;
exports.CloneType = CloneType;
const value_1 = require("./value");
/** Clones a Rest */
function CloneRest(schemas) {
    return schemas.map((schema) => CloneType(schema));
}
/** Clones a Type */
function CloneType(schema, options) {
    return options === undefined ? (0, value_1.Clone)(schema) : (0, value_1.Clone)({ ...options, ...schema });
}
