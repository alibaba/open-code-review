import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface BigIntOptions extends SchemaOptions {
    exclusiveMaximum?: bigint;
    exclusiveMinimum?: bigint;
    maximum?: bigint;
    minimum?: bigint;
    multipleOf?: bigint;
}
export interface TBigInt extends TSchema, BigIntOptions {
    [Kind]: 'BigInt';
    static: bigint;
    type: 'bigint';
}
/** `[JavaScript]` Creates a BigInt type */
export declare function BigInt(options?: BigIntOptions): TBigInt;
