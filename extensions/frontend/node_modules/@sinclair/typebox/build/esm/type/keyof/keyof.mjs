import { CreateType } from '../create/type.mjs';
import { Literal } from '../literal/index.mjs';
import { Number } from '../number/index.mjs';
import { Computed } from '../computed/index.mjs';
import { Ref } from '../ref/index.mjs';
import { KeyOfPropertyKeys } from './keyof-property-keys.mjs';
import { UnionEvaluated } from '../union/index.mjs';
import { KeyOfFromMappedResult } from './keyof-from-mapped-result.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsMappedResult, IsRef, IsComputed } from '../guard/kind.mjs';
// prettier-ignore
function FromComputed(target, parameters) {
    return Computed('KeyOf', [Computed(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return Computed('KeyOf', [Ref($ref)]);
}
// prettier-ignore
function KeyOfFromType(type, options) {
    const propertyKeys = KeyOfPropertyKeys(type);
    const propertyKeyTypes = KeyOfPropertyKeysToRest(propertyKeys);
    const result = UnionEvaluated(propertyKeyTypes);
    return CreateType(result, options);
}
// prettier-ignore
export function KeyOfPropertyKeysToRest(propertyKeys) {
    return propertyKeys.map(L => L === '[number]' ? Number() : Literal(L));
}
/** `[Json]` Creates a KeyOf type */
export function KeyOf(type, options) {
    return (IsComputed(type) ? FromComputed(type.target, type.parameters) : IsRef(type) ? FromRef(type.$ref) : IsMappedResult(type) ? KeyOfFromMappedResult(type, options) : KeyOfFromType(type, options));
}
