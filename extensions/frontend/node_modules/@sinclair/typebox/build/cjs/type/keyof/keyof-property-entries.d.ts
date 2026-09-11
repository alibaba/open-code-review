import { TSchema } from '../schema/index';
/**
 * `[Utility]` Resolves an array of keys and schemas from the given schema. This method is faster
 * than obtaining the keys and resolving each individually via indexing. This method was written
 * accellerate Intersect and Union encoding.
 */
export declare function KeyOfPropertyEntries(schema: TSchema): [key: string, schema: TSchema][];
