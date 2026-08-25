import { TypeBoxError } from '../../type/error/index';
export declare class ValueHashError extends TypeBoxError {
    readonly value: unknown;
    constructor(value: unknown);
}
/** Creates a FNV1A-64 non cryptographic hash of the given value */
export declare function Hash(value: unknown): bigint;
