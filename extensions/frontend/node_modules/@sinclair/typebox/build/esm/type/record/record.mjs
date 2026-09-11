import { CreateType } from '../create/type.mjs';
import { Kind, Hint } from '../symbols/index.mjs';
import { Never } from '../never/index.mjs';
import { Number } from '../number/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { String } from '../string/index.mjs';
import { Union } from '../union/index.mjs';
import { IsTemplateLiteralFinite } from '../template-literal/index.mjs';
import { PatternStringExact, PatternNumberExact, PatternNeverExact } from '../patterns/index.mjs';
import { IndexPropertyKeys } from '../indexed/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsUndefined } from '../guard/value.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsInteger, IsLiteral, IsAny, IsBoolean, IsNever, IsNumber, IsString, IsRegExp, IsTemplateLiteral, IsUnion } from '../guard/kind.mjs';
// ------------------------------------------------------------------
// RecordCreateFromPattern
// ------------------------------------------------------------------
// prettier-ignore
function RecordCreateFromPattern(pattern, T, options) {
    return CreateType({ [Kind]: 'Record', type: 'object', patternProperties: { [pattern]: T } }, options);
}
// ------------------------------------------------------------------
// RecordCreateFromKeys
// ------------------------------------------------------------------
// prettier-ignore
function RecordCreateFromKeys(K, T, options) {
    const result = {};
    for (const K2 of K)
        result[K2] = T;
    return _Object_(result, { ...options, [Hint]: 'Record' });
}
// prettier-ignore
function FromTemplateLiteralKey(K, T, options) {
    return (IsTemplateLiteralFinite(K)
        ? RecordCreateFromKeys(IndexPropertyKeys(K), T, options)
        : RecordCreateFromPattern(K.pattern, T, options));
}
// prettier-ignore
function FromUnionKey(key, type, options) {
    return RecordCreateFromKeys(IndexPropertyKeys(Union(key)), type, options);
}
// prettier-ignore
function FromLiteralKey(key, type, options) {
    return RecordCreateFromKeys([key.toString()], type, options);
}
// prettier-ignore
function FromRegExpKey(key, type, options) {
    return RecordCreateFromPattern(key.source, type, options);
}
// prettier-ignore
function FromStringKey(key, type, options) {
    const pattern = IsUndefined(key.pattern) ? PatternStringExact : key.pattern;
    return RecordCreateFromPattern(pattern, type, options);
}
// prettier-ignore
function FromAnyKey(_, type, options) {
    return RecordCreateFromPattern(PatternStringExact, type, options);
}
// prettier-ignore
function FromNeverKey(_key, type, options) {
    return RecordCreateFromPattern(PatternNeverExact, type, options);
}
// prettier-ignore
function FromBooleanKey(_key, type, options) {
    return _Object_({ true: type, false: type }, options);
}
// prettier-ignore
function FromIntegerKey(_key, type, options) {
    return RecordCreateFromPattern(PatternNumberExact, type, options);
}
// prettier-ignore
function FromNumberKey(_, type, options) {
    return RecordCreateFromPattern(PatternNumberExact, type, options);
}
// ------------------------------------------------------------------
// TRecordOrObject
// ------------------------------------------------------------------
/** `[Json]` Creates a Record type */
export function Record(key, type, options = {}) {
    // prettier-ignore
    return (IsUnion(key) ? FromUnionKey(key.anyOf, type, options) :
        IsTemplateLiteral(key) ? FromTemplateLiteralKey(key, type, options) :
            IsLiteral(key) ? FromLiteralKey(key.const, type, options) :
                IsBoolean(key) ? FromBooleanKey(key, type, options) :
                    IsInteger(key) ? FromIntegerKey(key, type, options) :
                        IsNumber(key) ? FromNumberKey(key, type, options) :
                            IsRegExp(key) ? FromRegExpKey(key, type, options) :
                                IsString(key) ? FromStringKey(key, type, options) :
                                    IsAny(key) ? FromAnyKey(key, type, options) :
                                        IsNever(key) ? FromNeverKey(key, type, options) :
                                            Never(options));
}
// ------------------------------------------------------------------
// Record Utilities
// ------------------------------------------------------------------
/** Gets the Records Pattern */
export function RecordPattern(record) {
    return globalThis.Object.getOwnPropertyNames(record.patternProperties)[0];
}
/** Gets the Records Key Type */
// prettier-ignore
export function RecordKey(type) {
    const pattern = RecordPattern(type);
    return (pattern === PatternStringExact ? String() :
        pattern === PatternNumberExact ? Number() :
            String({ pattern }));
}
/** Gets a Record Value Type */
// prettier-ignore
export function RecordValue(type) {
    return type.patternProperties[RecordPattern(type)];
}
