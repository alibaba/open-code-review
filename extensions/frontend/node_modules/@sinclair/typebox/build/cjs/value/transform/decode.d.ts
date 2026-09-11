import { TypeBoxError } from '../../type/error/index';
import { ValueError } from '../../errors/index';
import type { TSchema } from '../../type/schema/index';
export declare class TransformDecodeCheckError extends TypeBoxError {
    readonly schema: TSchema;
    readonly value: unknown;
    readonly error: ValueError;
    constructor(schema: TSchema, value: unknown, error: ValueError);
}
export declare class TransformDecodeError extends TypeBoxError {
    readonly schema: TSchema;
    readonly path: string;
    readonly value: unknown;
    readonly error: Error;
    constructor(schema: TSchema, path: string, value: unknown, error: Error);
}
/**
 * `[Internal]` Decodes the value and returns the result. This function requires that
 * the caller `Check` the value before use. Passing unchecked values may result in
 * undefined behavior. Refer to the `Value.Decode()` for implementation details.
 */
export declare function TransformDecode(schema: TSchema, references: TSchema[], value: unknown): unknown;
