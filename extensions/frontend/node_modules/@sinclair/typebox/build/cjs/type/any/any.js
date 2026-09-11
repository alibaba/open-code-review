"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Any = Any;
const index_1 = require("../create/index");
const index_2 = require("../symbols/index");
/** `[Json]` Creates an Any type */
function Any(options) {
    return (0, index_1.CreateType)({ [index_2.Kind]: 'Any' }, options);
}
