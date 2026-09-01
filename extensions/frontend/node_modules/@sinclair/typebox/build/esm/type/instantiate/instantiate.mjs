import { CloneType } from '../clone/type.mjs';
import { Unknown } from '../unknown/index.mjs';
import { ReadonlyOptional } from '../readonly-optional/index.mjs';
import { Readonly } from '../readonly/index.mjs';
import { Optional } from '../optional/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Record, RecordKey, RecordValue } from '../record/index.mjs';
import * as ValueGuard from '../guard/value.mjs';
import * as KindGuard from '../guard/kind.mjs';
// prettier-ignore
function FromConstructor(args, type) {
    type.parameters = FromTypes(args, type.parameters);
    type.returns = FromType(args, type.returns);
    return type;
}
// prettier-ignore
function FromFunction(args, type) {
    type.parameters = FromTypes(args, type.parameters);
    type.returns = FromType(args, type.returns);
    return type;
}
// prettier-ignore
function FromIntersect(args, type) {
    type.allOf = FromTypes(args, type.allOf);
    return type;
}
// prettier-ignore
function FromUnion(args, type) {
    type.anyOf = FromTypes(args, type.anyOf);
    return type;
}
// prettier-ignore
function FromTuple(args, type) {
    if (ValueGuard.IsUndefined(type.items))
        return type;
    type.items = FromTypes(args, type.items);
    return type;
}
// prettier-ignore
function FromArray(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromAsyncIterator(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromIterator(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromPromise(args, type) {
    type.item = FromType(args, type.item);
    return type;
}
// prettier-ignore
function FromObject(args, type) {
    const mappedProperties = FromProperties(args, type.properties);
    return { ...type, ..._Object_(mappedProperties) }; // retain options
}
// prettier-ignore
function FromRecord(args, type) {
    const mappedKey = FromType(args, RecordKey(type));
    const mappedValue = FromType(args, RecordValue(type));
    const result = Record(mappedKey, mappedValue);
    return { ...type, ...result }; // retain options
}
// prettier-ignore
function FromArgument(args, argument) {
    return argument.index in args ? args[argument.index] : Unknown();
}
// prettier-ignore
function FromProperty(args, type) {
    const isReadonly = KindGuard.IsReadonly(type);
    const isOptional = KindGuard.IsOptional(type);
    const mapped = FromType(args, type);
    return (isReadonly && isOptional ? ReadonlyOptional(mapped) :
        isReadonly && !isOptional ? Readonly(mapped) :
            !isReadonly && isOptional ? Optional(mapped) :
                mapped);
}
// prettier-ignore
function FromProperties(args, properties) {
    return globalThis.Object.getOwnPropertyNames(properties).reduce((result, key) => {
        return { ...result, [key]: FromProperty(args, properties[key]) };
    }, {});
}
// prettier-ignore
export function FromTypes(args, types) {
    return types.map(type => FromType(args, type));
}
// prettier-ignore
function FromType(args, type) {
    return (KindGuard.IsConstructor(type) ? FromConstructor(args, type) :
        KindGuard.IsFunction(type) ? FromFunction(args, type) :
            KindGuard.IsIntersect(type) ? FromIntersect(args, type) :
                KindGuard.IsUnion(type) ? FromUnion(args, type) :
                    KindGuard.IsTuple(type) ? FromTuple(args, type) :
                        KindGuard.IsArray(type) ? FromArray(args, type) :
                            KindGuard.IsAsyncIterator(type) ? FromAsyncIterator(args, type) :
                                KindGuard.IsIterator(type) ? FromIterator(args, type) :
                                    KindGuard.IsPromise(type) ? FromPromise(args, type) :
                                        KindGuard.IsObject(type) ? FromObject(args, type) :
                                            KindGuard.IsRecord(type) ? FromRecord(args, type) :
                                                KindGuard.IsArgument(type) ? FromArgument(args, type) :
                                                    type);
}
/** `[JavaScript]` Instantiates a type with the given parameters */
// prettier-ignore
export function Instantiate(type, args) {
    return FromType(args, CloneType(type));
}
