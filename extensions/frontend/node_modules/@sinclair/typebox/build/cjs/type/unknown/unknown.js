"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Unknown = Unknown;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates an Unknown type */
function Unknown(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Unknown' }, options);
}
