import { CreateType } from '../create/type.mjs';
import { Computed } from '../computed/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Union } from '../union/index.mjs';
import { Ref } from '../ref/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsIntersect, IsUnion, IsPromise, IsRef, IsComputed } from '../guard/kind.mjs';
// prettier-ignore
function FromComputed(target, parameters) {
    return Computed('Awaited', [Computed(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return Computed('Awaited', [Ref($ref)]);
}
// prettier-ignore
function FromIntersect(types) {
    return Intersect(FromRest(types));
}
// prettier-ignore
function FromUnion(types) {
    return Union(FromRest(types));
}
// prettier-ignore
function FromPromise(type) {
    return Awaited(type);
}
// prettier-ignore
function FromRest(types) {
    return types.map(type => Awaited(type));
}
/** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
export function Awaited(type, options) {
    return CreateType(IsComputed(type) ? FromComputed(type.target, type.parameters) : IsIntersect(type) ? FromIntersect(type.allOf) : IsUnion(type) ? FromUnion(type.anyOf) : IsPromise(type) ? FromPromise(type.item) : IsRef(type) ? FromRef(type.$ref) : type, options);
}
