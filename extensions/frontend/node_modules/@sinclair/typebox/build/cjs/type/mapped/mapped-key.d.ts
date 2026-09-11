import type { TSchema } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TMappedKey<T extends PropertyKey[] = PropertyKey[]> extends TSchema {
    [Kind]: 'MappedKey';
    static: T[number];
    keys: T;
}
export declare function MappedKey<T extends PropertyKey[]>(T: [...T]): TMappedKey<T>;
