import { TypeBoxError } from '../../type/error/index';
import { ValueError } from '../../errors/index';
import type { TSchema } from '../../type/schema/index';
export declare class TransformEncodeCheckError extends TypeBoxError {
    readonly schema: TSchema;
    readonly value: unknown;
    readonly error: ValueError;
    constructor(schema: TSchema, value: unknown, error: ValueError);
}
export declare class TransformEncodeError extends TypeBoxError {
    readonly schema: TSchema;
    readonly path: string;
    readonly value: unknown;
    readonly error: Error;
    constructor(schema: TSchema, path: string, value: unknown, error: Error);
}
/**
 * `[Internal]` Encodes the value and returns the result. This function expects the
 * caller to pass a statically checked value. This function does not check the encoded
 * result, meaning the result should be passed to `Check` before use. Refer to the
 * `Value.Encode()` function for implementation details.
 */
export declare function TransformEncode(schema: TSchema, references: TSchema[], value: unknown): unknown;
