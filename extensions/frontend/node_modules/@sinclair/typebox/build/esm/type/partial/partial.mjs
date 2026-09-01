import { CreateType } from '../create/type.mjs';
import { Computed } from '../computed/index.mjs';
import { Optional } from '../optional/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Union } from '../union/index.mjs';
import { Ref } from '../ref/index.mjs';
import { Discard } from '../discard/index.mjs';
import { TransformKind } from '../symbols/index.mjs';
import { PartialFromMappedResult } from './partial-from-mapped-result.mjs';
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
import * as KindGuard from '../guard/kind.mjs';
// prettier-ignore
function FromComputed(target, parameters) {
    return Computed('Partial', [Computed(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return Computed('Partial', [Ref($ref)]);
}
// prettier-ignore
function FromProperties(properties) {
    const partialProperties = {};
    for (const K of globalThis.Object.getOwnPropertyNames(properties))
        partialProperties[K] = Optional(properties[K]);
    return partialProperties;
}
// prettier-ignore
function FromObject(type, properties) {
    const options = Discard(type, [TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties);
    return _Object_(mappedProperties, options);
}
// prettier-ignore
function FromRest(types) {
    return types.map(type => PartialResolve(type));
}
// ------------------------------------------------------------------
// PartialResolve
// ------------------------------------------------------------------
// prettier-ignore
function PartialResolve(type) {
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
/** `[Json]` Constructs a type where all properties are optional */
export function Partial(type, options) {
    if (KindGuard.IsMappedResult(type)) {
        return PartialFromMappedResult(type, options);
    }
    else {
        // special: mapping types require overridable options
        return CreateType({ ...PartialResolve(type), ...options });
    }
}
