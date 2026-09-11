import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { TUnion } from './union-type.mjs';
export declare function UnionCreate<T extends TSchema[]>(T: [...T], options?: SchemaOptions): TUnion<T>;
