"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.SetIncludes = SetIncludes;
exports.SetIsSubset = SetIsSubset;
exports.SetDistinct = SetDistinct;
exports.SetIntersect = SetIntersect;
exports.SetUnion = SetUnion;
exports.SetComplement = SetComplement;
exports.SetIntersectMany = SetIntersectMany;
exports.SetUnionMany = SetUnionMany;
/** Returns true if element right is in the set of left */
// prettier-ignore
function SetIncludes(T, S) {
    return T.includes(S);
}
/** Returns true if left is a subset of right */
function SetIsSubset(T, S) {
    return T.every((L) => SetIncludes(S, L));
}
/** Returns a distinct set of elements */
function SetDistinct(T) {
    return [...new Set(T)];
}
/** Returns the Intersect of the given sets */
function SetIntersect(T, S) {
    return T.filter((L) => S.includes(L));
}
/** Returns the Union of the given sets */
function SetUnion(T, S) {
    return [...T, ...S];
}
/** Returns the Complement by omitting elements in T that are in S */
// prettier-ignore
function SetComplement(T, S) {
    return T.filter(L => !S.includes(L));
}
// prettier-ignore
function SetIntersectManyResolve(T, Init) {
    return T.reduce((Acc, L) => {
        return SetIntersect(Acc, L);
    }, Init);
}
// prettier-ignore
function SetIntersectMany(T) {
    return (T.length === 1
        ? T[0]
        // Use left to initialize the accumulator for resolve
        : T.length > 1
            ? SetIntersectManyResolve(T.slice(1), T[0])
            : []);
}
/** Returns the Union of multiple sets */
function SetUnionMany(T) {
    const Acc = [];
    for (const L of T)
        Acc.push(...L);
    return Acc;
}
