import type { TSchema, SchemaOptions } from '../schema/index';
import { type TLiteral } from '../literal/index';
import { Kind, Hint } from '../symbols/index';
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
