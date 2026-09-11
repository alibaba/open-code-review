import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { type TLiteral } from '../literal/index.mjs';
import { Kind, Hint } from '../symbols/index.mjs';
export type TEnumRecord = Record<TEnumKey, TEnumValue>;
export type TEnumValue = string | number;
export type TEnumKey = string;
export interface TEnum<T extends Record<string, string | number> = Record<string, string | number>> extends TSchema {
    [Kind]: 'Union';
    [Hint]: 'Enum';
    static: T[keyof T];
    anyOf: TLiteral<T[keyof T]>[];
}
/** `[Json]` Creates a Enum type */
export declare function Enum<V extends TEnumValue, T extends Record<TEnumKey, V>>(item: T, options?: SchemaOptions): TEnum<T>;
