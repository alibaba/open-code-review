import { CreateType } from '../create/type.mjs';
import { Computed } from '../computed/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Union } from '../union/index.mjs';
import { Ref } from '../ref/index.mjs';
import { OptionalKind, TransformKind } from '../symbols/index.mjs';
import { Discard } from '../discard/index.mjs';
import { RequiredFromMappedResult } from './required-from-mapped-result.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import * as KindGuard from '../guard/kind.mjs';
// prettier-ignore
function FromComputed(target, parameters) {
    return Computed('Required', [Computed(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return Computed('Required', [Ref($ref)]);
}
// prettier-ignore
function FromProperties(properties) {
    const requiredProperties = {};
    for (const K of globalThis.Object.getOwnPropertyNames(properties))
        requiredProperties[K] = Discard(properties[K], [OptionalKind]);
    return requiredProperties;
}
// prettier-ignore
function FromObject(type, properties) {
    const options = Discard(type, [TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties);
    return _Object_(mappedProperties, options);
}
// prettier-ignore
function FromRest(types) {
    return types.map(type => RequiredResolve(type));
}
// ------------------------------------------------------------------
// RequiredResolve
// ------------------------------------------------------------------
// prettier-ignore
function RequiredResolve(type) {
    return (
    // Mappable
    KindGuard.IsComputed(type) ? FromComputed(type.target, type.parameters) :
        KindGuard.IsRef(type) ? FromRef(type.$ref) :
            KindGuard.IsIntersect(type) ? Intersect(FromRest(type.allOf)) :
                KindGuard.IsUnion(type) ? Union(FromRest(type.anyOf)) :
                    KindGuard.IsObject(type) ? FromObject(type, type.properties) :
                        // Intrinsic
                        KindGuard.IsBigInt(type) ? type :
                            KindGuard.IsBoolean(type) ? type :
                                KindGuard.IsInteger(type) ? type :
                                    KindGuard.IsLiteral(type) ? type :
                                        KindGuard.IsNull(type) ? type :
                                            KindGuard.IsNumber(type) ? type :
                                                KindGuard.IsString(type) ? type :
                                                    KindGuard.IsSymbol(type) ? type :
                                                        KindGuard.IsUndefined(type) ? type :
                                                            // Passthrough
                                                            _Object_({}));
}
/** `[Json]` Constructs a type where all properties are required */
export function Required(type, options) {
    if (KindGuard.IsMappedResult(type)) {
        return RequiredFromMappedResult(type, options);
    }
    else {
        // special: mapping types require overridable options
        return CreateType({ ...RequiredResolve(type), ...options });
    }
}
