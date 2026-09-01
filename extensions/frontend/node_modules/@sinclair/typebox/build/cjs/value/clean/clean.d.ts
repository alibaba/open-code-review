import type { TSchema } from '../../type/schema/index';
/** `[Mutable]` Removes excess properties from a value and returns the result. This function does not check the value and returns an unknown type. You should Check the result before use. Clean is a mutable operation. To avoid mutation, Clone the value first. */
export declare function Clean(schema: TSchema, references: TSchema[], value: unknown): unknown;
/** `[Mutable]` Removes excess properties from a value and returns the result. This function does not check the value and returns an unknown type. You should Check the result before use. Clean is a mutable operation. To avoid mutation, Clone the value first. */
export declare function Clean(schema: TSchema, value: unknown): unknown;
