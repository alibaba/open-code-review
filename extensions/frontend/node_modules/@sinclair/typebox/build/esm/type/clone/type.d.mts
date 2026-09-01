import { TSchema, SchemaOptions } from '../schema/index.mjs';
/** Clones a Rest */
export declare function CloneRest<T extends TSchema[]>(schemas: T): T;
/** Clones a Type */
export declare function CloneType<T extends TSchema>(schema: T, options?: SchemaOptions): T;
