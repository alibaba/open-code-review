"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Intersect = Intersect;
const type_1 = require("../create/type");
const index_1 = require("../never/index");
const intersect_create_1 = require("./intersect-create");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
/** `[Json]` Creates an evaluated Intersect type */
function Intersect(types, options) {
    if (types.length === 1)
        return (0, type_1.CreateType)(types[0], options);
    if (types.length === 0)
        return (0, index_1.Never)(options);
    if (types.some((schema) => (0, kind_1.IsTransform)(schema)))
        throw new Error('Cannot intersect transform types');
    return (0, intersect_create_1.IntersectCreate)(types, options);
}
