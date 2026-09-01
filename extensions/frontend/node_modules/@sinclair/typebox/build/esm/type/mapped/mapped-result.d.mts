import type { TSchema } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TMappedResult<T extends TProperties = TProperties> extends TSchema {
    [Kind]: 'MappedResult';
    properties: T;
    static: unknown;
}
export declare function MappedResult<T extends TProperties>(properties: T): TMappedResult<T>;
