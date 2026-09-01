import type { AssertRest, Evaluate } from '../helpers/index.mjs';
import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { type TAny } from '../any/index.mjs';
import { type TBigInt } from '../bigint/index.mjs';
import { type TDate } from '../date/index.mjs';
import { type TFunction } from '../function/index.mjs';
import { type TLiteral } from '../literal/index.mjs';
import { type TNever } from '../never/index.mjs';
import { type TNull } from '../null/index.mjs';
import { type TObject } from '../object/index.mjs';
import { type TSymbol } from '../symbol/index.mjs';
import { type TTuple } from '../tuple/index.mjs';
import { type TReadonly } from '../readonly/index.mjs';
import { type TUndefined } from '../undefined/index.mjs';
import { type TUint8Array } from '../uint8array/index.mjs';
import { type TUnknown } from '../unknown/index.mjs';
type TFromArray<T extends readonly unknown[]> = T extends readonly [infer L extends unknown, ...infer R extends unknown[]] ? [FromValue<L, false>, ...TFromArray<R>] : T;
type TFromProperties<T extends Record<PropertyKey, unknown>> = {
    -readonly [K in keyof T]: FromValue<T[K], false> extends infer R extends TSchema ? TReadonly<R> : TReadonly<TNever>;
};
type TConditionalReadonly<T extends TSchema, Root extends boolean> = Root extends true ? T : TReadonly<T>;
type FromValue<T, Root extends boolean> = T extends AsyncIterableIterator<unknown> ? TConditionalReadonly<TAny, Root> : T extends IterableIterator<unknown> ? TConditionalReadonly<TAny, Root> : T extends readonly unknown[] ? TReadonly<TTuple<AssertRest<TFromArray<T>>>> : T extends Uint8Array ? TUint8Array : T extends Date ? TDate : T extends Record<PropertyKey, unknown> ? TConditionalReadonly<TObject<Evaluate<TFromProperties<T>>>, Root> : T extends Function ? TConditionalReadonly<TFunction<[], TUnknown>, Root> : T extends undefined ? TUndefined : T extends null ? TNull : T extends symbol ? TSymbol : T extends number ? TLiteral<T> : T extends boolean ? TLiteral<T> : T extends string ? TLiteral<T> : T extends bigint ? TBigInt : TObject<{}>;
declare function FromValue<T, Root extends boolean>(value: T, root: Root): FromValue<T, Root>;
export type TConst<T> = FromValue<T, true>;
/** `[JavaScript]` Creates a readonly const type from the given value. */
export declare function Const</* const (not supported in 4.0) */ T>(T: T, options?: SchemaOptions): TConst<T>;
export {};
