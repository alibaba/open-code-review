"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Union = Union;
const index_1 = require("../never/index");
const type_1 = require("../create/type");
const union_create_1 = require("./union-create");
/** `[Json]` Creates a Union type */
function Union(types, options) {
    // prettier-ignore
    return (types.length === 0 ? (0, index_1.Never)(options) :
        types.length === 1 ? (0, type_1.CreateType)(types[0], options) :
            (0, union_create_1.UnionCreate)(types, options));
}
