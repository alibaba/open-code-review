"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Rest = Rest;
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function RestResolve(T) {
    return ((0, kind_1.IsIntersect)(T) ? T.allOf :
        (0, kind_1.IsUnion)(T) ? T.anyOf :
            (0, kind_1.IsTuple)(T) ? T.items ?? [] :
                []);
}
/** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
function Rest(T) {
    return RestResolve(T);
}
