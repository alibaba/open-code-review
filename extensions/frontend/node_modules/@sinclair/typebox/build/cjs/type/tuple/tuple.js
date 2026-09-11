"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Tuple = Tuple;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Tuple type */
function Tuple(types, options) {
    // prettier-ignore
    return (0, type_1.CreateType)(types.length > 0 ?
        { [index_1.Kind]: 'Tuple', type: 'array', items: types, additionalItems: false, minItems: types.length, maxItems: types.length } :
        { [index_1.Kind]: 'Tuple', type: 'array', minItems: types.length, maxItems: types.length }, options);
}
