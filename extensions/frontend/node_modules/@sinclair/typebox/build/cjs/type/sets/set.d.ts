export type TSetIncludes<T extends PropertyKey[], S extends PropertyKey> = (T extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? S extends L ? true : TSetIncludes<R, S> : false);
/** Returns true if element right is in the set of left */
export declare function SetIncludes<T extends PropertyKey[], S extends PropertyKey>(T: [...T], S: S): TSetIncludes<T, S>;
export type TSetIsSubset<T extends PropertyKey[], S extends PropertyKey[]> = (T extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TSetIncludes<S, L> extends true ? TSetIsSubset<R, S> : false : true);
/** Returns true if left is a subset of right */
export declare function SetIsSubset<T extends PropertyKey[], S extends PropertyKey[]>(T: [...T], S: [...S]): TSetIsSubset<T, S>;
export type TSetDistinct<T extends PropertyKey[], Acc extends PropertyKey[] = []> = T extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TSetIncludes<Acc, L> extends false ? TSetDistinct<R, [...Acc, L]> : TSetDistinct<R, [...Acc]> : Acc;
/** Returns a distinct set of elements */
export declare function SetDistinct<T extends PropertyKey[]>(T: [...T]): TSetDistinct<T>;
export type TSetIntersect<T extends PropertyKey[], S extends PropertyKey[], Acc extends PropertyKey[] = []> = (T extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TSetIncludes<S, L> extends true ? TSetIntersect<R, S, [...Acc, L]> : TSetIntersect<R, S, [...Acc]> : Acc);
/** Returns the Intersect of the given sets */
export declare function SetIntersect<T extends PropertyKey[], S extends PropertyKey[]>(T: [...T], S: [...S]): TSetIntersect<T, S>;
export type TSetUnion<T extends PropertyKey[], S extends PropertyKey[]> = ([
    ...T,
    ...S
]);
/** Returns the Union of the given sets */
export declare function SetUnion<T extends PropertyKey[], S extends PropertyKey[]>(T: [...T], S: [...S]): TSetUnion<T, S>;
export type TSetComplement<T extends PropertyKey[], S extends PropertyKey[], Acc extends PropertyKey[] = []> = (T extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TSetIncludes<S, L> extends true ? TSetComplement<R, S, [...Acc]> : TSetComplement<R, S, [...Acc, L]> : Acc);
/** Returns the Complement by omitting elements in T that are in S */
export declare function SetComplement<T extends PropertyKey[], S extends PropertyKey[]>(T: [...T], S: [...S]): TSetComplement<T, S>;
type TSetIntersectManyResolve<T extends PropertyKey[][], Acc extends PropertyKey[]> = (T extends [infer L extends PropertyKey[], ...infer R extends PropertyKey[][]] ? TSetIntersectManyResolve<R, TSetIntersect<Acc, L>> : Acc);
export type TSetIntersectMany<T extends PropertyKey[][]> = (T extends [infer L extends PropertyKey[]] ? L : T extends [infer L extends PropertyKey[], ...infer R extends PropertyKey[][]] ? TSetIntersectManyResolve<R, L> : []);
export declare function SetIntersectMany<T extends PropertyKey[][]>(T: [...T]): TSetIntersectMany<T>;
export type TSetUnionMany<T extends PropertyKey[][], Acc extends PropertyKey[] = []> = (T extends [infer L extends PropertyKey[], ...infer R extends PropertyKey[][]] ? TSetUnionMany<R, TSetUnion<Acc, L>> : Acc);
/** Returns the Union of multiple sets */
export declare function SetUnionMany<T extends PropertyKey[][]>(T: [...T]): TSetUnionMany<T>;
export {};
