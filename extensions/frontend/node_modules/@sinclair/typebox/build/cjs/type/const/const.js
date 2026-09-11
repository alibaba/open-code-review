"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Const = Const;
const index_1 = require("../any/index");
const index_2 = require("../bigint/index");
const index_3 = require("../date/index");
const index_4 = require("../function/index");
const index_5 = require("../literal/index");
const index_6 = require("../null/index");
const index_7 = require("../object/index");
const index_8 = require("../symbol/index");
const index_9 = require("../tuple/index");
const index_10 = require("../readonly/index");
const index_11 = require("../undefined/index");
const index_12 = require("../uint8array/index");
const index_13 = require("../unknown/index");
const index_14 = require("../create/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const value_1 = require("../guard/value");
// prettier-ignore
function FromArray(T) {
    return T.map(L => FromValue(L, false));
}
// prettier-ignore
function FromProperties(value) {
    const Acc = {};
    for (const K of globalThis.Object.getOwnPropertyNames(value))
        Acc[K] = (0, index_10.Readonly)(FromValue(value[K], false));
    return Acc;
}
function ConditionalReadonly(T, root) {
    return (root === true ? T : (0, index_10.Readonly)(T));
}
// prettier-ignore
function FromValue(value, root) {
    return ((0, value_1.IsAsyncIterator)(value) ? ConditionalReadonly((0, index_1.Any)(), root) :
        (0, value_1.IsIterator)(value) ? ConditionalReadonly((0, index_1.Any)(), root) :
            (0, value_1.IsArray)(value) ? (0, index_10.Readonly)((0, index_9.Tuple)(FromArray(value))) :
                (0, value_1.IsUint8Array)(value) ? (0, index_12.Uint8Array)() :
                    (0, value_1.IsDate)(value) ? (0, index_3.Date)() :
                        (0, value_1.IsObject)(value) ? ConditionalReadonly((0, index_7.Object)(FromProperties(value)), root) :
                            (0, value_1.IsFunction)(value) ? ConditionalReadonly((0, index_4.Function)([], (0, index_13.Unknown)()), root) :
                                (0, value_1.IsUndefined)(value) ? (0, index_11.Undefined)() :
                                    (0, value_1.IsNull)(value) ? (0, index_6.Null)() :
                                        (0, value_1.IsSymbol)(value) ? (0, index_8.Symbol)() :
                                            (0, value_1.IsBigInt)(value) ? (0, index_2.BigInt)() :
                                                (0, value_1.IsNumber)(value) ? (0, index_5.Literal)(value) :
                                                    (0, value_1.IsBoolean)(value) ? (0, index_5.Literal)(value) :
                                                        (0, value_1.IsString)(value) ? (0, index_5.Literal)(value) :
                                                            (0, index_7.Object)({}));
}
/** `[JavaScript]` Creates a readonly const type from the given value. */
function Const(T, options) {
    return (0, index_14.CreateType)(FromValue(T, true), options);
}
