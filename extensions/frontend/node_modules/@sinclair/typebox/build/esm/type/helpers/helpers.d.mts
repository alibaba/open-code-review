import type { TSchema } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import type { TNever } from '../never/index.mjs';
export type TupleToIntersect<T extends any[]> = T extends [infer I] ? I : T extends [infer I, ...infer R] ? I & TupleToIntersect<R> : never;
export type TupleToUnion<T extends any[]> = {
    [K in keyof T]: T[K];
}[number];
export type UnionToIntersect<U> = (U extends unknown ? (arg: U) => 0 : never) extends (arg: infer I) => 0 ? I : never;
export type UnionLast<U> = UnionToIntersect<U extends unknown ? (x: U) => 0 : never> extends (x: infer L) => 0 ? L : never;
export type UnionToTuple<U, Acc extends unknown[] = [], R = UnionLast<U>> = [U] extends [never] ? Acc : UnionToTuple<Exclude<U, R>, [Extract<U, R>, ...Acc]>;
export type Trim<T> = T extends `${' '}${infer U}` ? Trim<U> : T extends `${infer U}${' '}` ? Trim<U> : T;
export type Assert<T, E> = T extends E ? T : never;
export type Evaluate<T> = T extends infer O ? {
    [K in keyof O]: O[K];
} : never;
export type Ensure<T> = T extends infer U ? U : never;
export type EmptyString = '';
export type ZeroString = '0';
type IncrementBase = {
    m: '9';
    t: '01';
    '0': '1';
    '1': '2';
    '2': '3';
    '3': '4';
    '4': '5';
    '5': '6';
    '6': '7';
    '7': '8';
    '8': '9';
    '9': '0';
};
type IncrementTake<T extends keyof IncrementBase> = IncrementBase[T];
type IncrementStep<T extends string> = T extends IncrementBase['m'] ? IncrementBase['t'] : T extends `${infer L extends keyof IncrementBase}${infer R}` ? L extends IncrementBase['m'] ? `${IncrementTake<L>}${IncrementStep<R>}` : `${IncrementTake<L>}${R}` : never;
type IncrementReverse<T extends string> = T extends `${infer L}${infer R}` ? `${IncrementReverse<R>}${L}` : T;
export type TIncrement<T extends string> = IncrementReverse<IncrementStep<IncrementReverse<T>>>;
/** Increments the given string value + 1 */
export declare function Increment<T extends string>(T: T): TIncrement<T>;
export type AssertProperties<T> = T extends TProperties ? T : TProperties;
export type AssertRest<T, E extends TSchema[] = TSchema[]> = T extends E ? T : [];
export type AssertType<T, E extends TSchema = TSchema> = T extends E ? T : TNever;
export {};
