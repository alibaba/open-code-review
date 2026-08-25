import type { TSchema } from '../schema/index';
import type { TProperties } from '../object/index';
import { Kind } from '../symbols/index';
export interface TMappedResult<T extends TProperties = TProperties> extends TSchema {
    [Kind]: 'MappedResult';
    properties: T;
    static: unknown;
}
export declare function MappedResult<T extends TProperties>(properties: T): TMappedResult<T>;
