import { Any } from '../any/index.mjs';
import { BigInt } from '../bigint/index.mjs';
import { Date } from '../date/index.mjs';
import { Function as FunctionType } from '../function/index.mjs';
import { Literal } from '../literal/index.mjs';
import { Null } from '../null/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Symbol } from '../symbol/index.mjs';
import { Tuple } from '../tuple/index.mjs';
import { Readonly } from '../readonly/index.mjs';
import { Undefined } from '../undefined/index.mjs';
import { Uint8Array } from '../uint8array/index.mjs';
import { Unknown } from '../unknown/index.mjs';
import { CreateType } from '../create/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsArray, IsNumber, IsBigInt, IsUint8Array, IsDate, IsIterator, IsObject, IsAsyncIterator, IsFunction, IsUndefined, IsNull, IsSymbol, IsBoolean, IsString } from '../guard/value.mjs';
// prettier-ignore
function FromArray(T) {
    return T.map(L => FromValue(L, false));
}
// prettier-ignore
function FromProperties(value) {
    const Acc = {};
    for (const K of globalThis.Object.getOwnPropertyNames(value))
        Acc[K] = Readonly(FromValue(value[K], false));
    return Acc;
}
function ConditionalReadonly(T, root) {
    return (root === true ? T : Readonly(T));
}
// prettier-ignore
function FromValue(value, root) {
    return (IsAsyncIterator(value) ? ConditionalReadonly(Any(), root) :
        IsIterator(value) ? ConditionalReadonly(Any(), root) :
            IsArray(value) ? Readonly(Tuple(FromArray(value))) :
                IsUint8Array(value) ? Uint8Array() :
                    IsDate(value) ? Date() :
                        IsObject(value) ? ConditionalReadonly(_Object_(FromProperties(value)), root) :
                            IsFunction(value) ? ConditionalReadonly(FunctionType([], Unknown()), root) :
                                IsUndefined(value) ? Undefined() :
                                    IsNull(value) ? Null() :
                                        IsSymbol(value) ? Symbol() :
                                            IsBigInt(value) ? BigInt() :
                                                IsNumber(value) ? Literal(value) :
                                                    IsBoolean(value) ? Literal(value) :
                                                        IsString(value) ? Literal(value) :
                                                            _Object_({}));
}
/** `[JavaScript]` Creates a readonly const type from the given value. */
export function Const(T, options) {
    return CreateType(FromValue(T, true), options);
}
