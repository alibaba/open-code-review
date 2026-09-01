"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Record = Record;
exports.RecordPattern = RecordPattern;
exports.RecordKey = RecordKey;
exports.RecordValue = RecordValue;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
const index_2 = require("../never/index");
const index_3 = require("../number/index");
const index_4 = require("../object/index");
const index_5 = require("../string/index");
const index_6 = require("../union/index");
const index_7 = require("../template-literal/index");
const index_8 = require("../patterns/index");
const index_9 = require("../indexed/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const value_1 = require("../guard/value");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// ------------------------------------------------------------------
// RecordCreateFromPattern
// ------------------------------------------------------------------
// prettier-ignore
function RecordCreateFromPattern(pattern, T, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Record', type: 'object', patternProperties: { [pattern]: T } }, options);
}
// ------------------------------------------------------------------
// RecordCreateFromKeys
// ------------------------------------------------------------------
// prettier-ignore
function RecordCreateFromKeys(K, T, options) {
    const result = {};
    for (const K2 of K)
        result[K2] = T;
    return (0, index_4.Object)(result, { ...options, [index_1.Hint]: 'Record' });
}
// prettier-ignore
function FromTemplateLiteralKey(K, T, options) {
    return ((0, index_7.IsTemplateLiteralFinite)(K)
        ? RecordCreateFromKeys((0, index_9.IndexPropertyKeys)(K), T, options)
        : RecordCreateFromPattern(K.pattern, T, options));
}
// prettier-ignore
function FromUnionKey(key, type, options) {
    return RecordCreateFromKeys((0, index_9.IndexPropertyKeys)((0, index_6.Union)(key)), type, options);
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
    const pattern = (0, value_1.IsUndefined)(key.pattern) ? index_8.PatternStringExact : key.pattern;
    return RecordCreateFromPattern(pattern, type, options);
}
// prettier-ignore
function FromAnyKey(_, type, options) {
    return RecordCreateFromPattern(index_8.PatternStringExact, type, options);
}
// prettier-ignore
function FromNeverKey(_key, type, options) {
    return RecordCreateFromPattern(index_8.PatternNeverExact, type, options);
}
// prettier-ignore
function FromBooleanKey(_key, type, options) {
    return (0, index_4.Object)({ true: type, false: type }, options);
}
// prettier-ignore
function FromIntegerKey(_key, type, options) {
    return RecordCreateFromPattern(index_8.PatternNumberExact, type, options);
}
// prettier-ignore
function FromNumberKey(_, type, options) {
    return RecordCreateFromPattern(index_8.PatternNumberExact, type, options);
}
// ------------------------------------------------------------------
// TRecordOrObject
// ------------------------------------------------------------------
/** `[Json]` Creates a Record type */
function Record(key, type, options = {}) {
    // prettier-ignore
    return ((0, kind_1.IsUnion)(key) ? FromUnionKey(key.anyOf, type, options) :
        (0, kind_1.IsTemplateLiteral)(key) ? FromTemplateLiteralKey(key, type, options) :
            (0, kind_1.IsLiteral)(key) ? FromLiteralKey(key.const, type, options) :
                (0, kind_1.IsBoolean)(key) ? FromBooleanKey(key, type, options) :
                    (0, kind_1.IsInteger)(key) ? FromIntegerKey(key, type, options) :
                        (0, kind_1.IsNumber)(key) ? FromNumberKey(key, type, options) :
                            (0, kind_1.IsRegExp)(key) ? FromRegExpKey(key, type, options) :
                                (0, kind_1.IsString)(key) ? FromStringKey(key, type, options) :
                                    (0, kind_1.IsAny)(key) ? FromAnyKey(key, type, options) :
                                        (0, kind_1.IsNever)(key) ? FromNeverKey(key, type, options) :
                                            (0, index_2.Never)(options));
}
// ------------------------------------------------------------------
// Record Utilities
// ------------------------------------------------------------------
/** Gets the Records Pattern */
function RecordPattern(record) {
    return globalThis.Object.getOwnPropertyNames(record.patternProperties)[0];
}
/** Gets the Records Key Type */
// prettier-ignore
function RecordKey(type) {
    const pattern = RecordPattern(type);
    return (pattern === index_8.PatternStringExact ? (0, index_5.String)() :
        pattern === index_8.PatternNumberExact ? (0, index_3.Number)() :
            (0, index_5.String)({ pattern }));
}
/** Gets a Record Value Type */
// prettier-ignore
function RecordValue(type) {
    return type.patternProperties[RecordPattern(type)];
}
