import type { TSchema } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TMappedKey<T extends PropertyKey[] = PropertyKey[]> extends TSchema {
    [Kind]: 'MappedKey';
    static: T[number];
    keys: T;
}
export declare function MappedKey<T extends PropertyKey[]>(T: [...T]): TMappedKey<T>;
