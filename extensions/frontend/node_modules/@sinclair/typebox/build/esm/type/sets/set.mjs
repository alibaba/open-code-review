/** Returns true if element right is in the set of left */
// prettier-ignore
export function SetIncludes(T, S) {
    return T.includes(S);
}
/** Returns true if left is a subset of right */
export function SetIsSubset(T, S) {
    return T.every((L) => SetIncludes(S, L));
}
/** Returns a distinct set of elements */
export function SetDistinct(T) {
    return [...new Set(T)];
}
/** Returns the Intersect of the given sets */
export function SetIntersect(T, S) {
    return T.filter((L) => S.includes(L));
}
/** Returns the Union of the given sets */
export function SetUnion(T, S) {
    return [...T, ...S];
}
/** Returns the Complement by omitting elements in T that are in S */
// prettier-ignore
export function SetComplement(T, S) {
    return T.filter(L => !S.includes(L));
}
// prettier-ignore
function SetIntersectManyResolve(T, Init) {
    return T.reduce((Acc, L) => {
        return SetIntersect(Acc, L);
    }, Init);
}
// prettier-ignore
export function SetIntersectMany(T) {
    return (T.length === 1
        ? T[0]
        // Use left to initialize the accumulator for resolve
        : T.length > 1
            ? SetIntersectManyResolve(T.slice(1), T[0])
            : []);
}
/** Returns the Union of multiple sets */
export function SetUnionMany(T) {
    const Acc = [];
    for (const L of T)
        Acc.push(...L);
    return Acc;
}
