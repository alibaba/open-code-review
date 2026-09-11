"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.MappedFunctionReturnType = MappedFunctionReturnType;
exports.Mapped = Mapped;
const index_1 = require("../symbols/index");
const index_2 = require("../discard/index");
// evaluation types
const index_3 = require("../array/index");
const index_4 = require("../async-iterator/index");
const index_5 = require("../constructor/index");
const index_6 = require("../function/index");
const index_7 = require("../indexed/index");
const index_8 = require("../intersect/index");
const index_9 = require("../iterator/index");
const index_10 = require("../literal/index");
const index_11 = require("../object/index");
const index_12 = require("../optional/index");
const index_13 = require("../promise/index");
const index_14 = require("../readonly/index");
const index_15 = require("../tuple/index");
const index_16 = require("../union/index");
// operator
const index_17 = require("../sets/index");
// mapping types
const mapped_result_1 = require("./mapped-result");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromMappedResult(K, P) {
    return (K in P
        ? FromSchemaType(K, P[K])
        : (0, mapped_result_1.MappedResult)(P));
}
// prettier-ignore
function MappedKeyToKnownMappedResultProperties(K) {
    return { [K]: (0, index_10.Literal)(K) };
}
// prettier-ignore
function MappedKeyToUnknownMappedResultProperties(P) {
    const Acc = {};
    for (const L of P)
        Acc[L] = (0, index_10.Literal)(L);
    return Acc;
}
// prettier-ignore
function MappedKeyToMappedResultProperties(K, P) {
    return ((0, index_17.SetIncludes)(P, K)
        ? MappedKeyToKnownMappedResultProperties(K)
        : MappedKeyToUnknownMappedResultProperties(P));
}
// prettier-ignore
function FromMappedKey(K, P) {
    const R = MappedKeyToMappedResultProperties(K, P);
    return FromMappedResult(K, R);
}
// prettier-ignore
function FromRest(K, T) {
    return T.map(L => FromSchemaType(K, L));
}
// prettier-ignore
function FromProperties(K, T) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(T))
        Acc[K2] = FromSchemaType(K, T[K2]);
    return Acc;
}
// prettier-ignore
function FromSchemaType(K, T) {
    // required to retain user defined options for mapped type
    const options = { ...T };
    return (
    // unevaluated modifier types
    (0, kind_1.IsOptional)(T) ? (0, index_12.Optional)(FromSchemaType(K, (0, index_2.Discard)(T, [index_1.OptionalKind]))) :
        (0, kind_1.IsReadonly)(T) ? (0, index_14.Readonly)(FromSchemaType(K, (0, index_2.Discard)(T, [index_1.ReadonlyKind]))) :
            // unevaluated mapped types
            (0, kind_1.IsMappedResult)(T) ? FromMappedResult(K, T.properties) :
                (0, kind_1.IsMappedKey)(T) ? FromMappedKey(K, T.keys) :
                    // unevaluated types
                    (0, kind_1.IsConstructor)(T) ? (0, index_5.Constructor)(FromRest(K, T.parameters), FromSchemaType(K, T.returns), options) :
                        (0, kind_1.IsFunction)(T) ? (0, index_6.Function)(FromRest(K, T.parameters), FromSchemaType(K, T.returns), options) :
                            (0, kind_1.IsAsyncIterator)(T) ? (0, index_4.AsyncIterator)(FromSchemaType(K, T.items), options) :
                                (0, kind_1.IsIterator)(T) ? (0, index_9.Iterator)(FromSchemaType(K, T.items), options) :
                                    (0, kind_1.IsIntersect)(T) ? (0, index_8.Intersect)(FromRest(K, T.allOf), options) :
                                        (0, kind_1.IsUnion)(T) ? (0, index_16.Union)(FromRest(K, T.anyOf), options) :
                                            (0, kind_1.IsTuple)(T) ? (0, index_15.Tuple)(FromRest(K, T.items ?? []), options) :
                                                (0, kind_1.IsObject)(T) ? (0, index_11.Object)(FromProperties(K, T.properties), options) :
                                                    (0, kind_1.IsArray)(T) ? (0, index_3.Array)(FromSchemaType(K, T.items), options) :
                                                        (0, kind_1.IsPromise)(T) ? (0, index_13.Promise)(FromSchemaType(K, T.item), options) :
                                                            T);
}
// prettier-ignore
function MappedFunctionReturnType(K, T) {
    const Acc = {};
    for (const L of K)
        Acc[L] = FromSchemaType(L, T);
    return Acc;
}
/** `[Json]` Creates a Mapped object type */
function Mapped(key, map, options) {
    const K = (0, kind_1.IsSchema)(key) ? (0, index_7.IndexPropertyKeys)(key) : key;
    const RT = map({ [index_1.Kind]: 'MappedKey', keys: K });
    const R = MappedFunctionReturnType(K, RT);
    return (0, index_11.Object)(R, options);
}
