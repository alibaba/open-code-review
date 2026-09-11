import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface Uint8ArrayOptions extends SchemaOptions {
    maxByteLength?: number;
    minByteLength?: number;
}
export interface TUint8Array extends TSchema, Uint8ArrayOptions {
    [Kind]: 'Uint8Array';
    static: Uint8Array;
    type: 'uint8array';
}
/** `[JavaScript]` Creates a Uint8Array type */
export declare function Uint8Array(options?: Uint8ArrayOptions): TUint8Array;
