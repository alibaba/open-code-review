import type { TSchema } from '../../type/schema/index';
/** `[Mutable]` Generates missing properties on a value using default schema annotations if available. This function does not check the value and returns an unknown type. You should Check the result before use. Default is a mutable operation. To avoid mutation, Clone the value first. */
export declare function Default(schema: TSchema, references: TSchema[], value: unknown): unknown;
/** `[Mutable]` Generates missing properties on a value using default schema annotations if available. This function does not check the value and returns an unknown type. You should Check the result before use. Default is a mutable operation. To avoid mutation, Clone the value first. */
export declare function Default(schema: TSchema, value: unknown): unknown;
