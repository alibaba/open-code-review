import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { TFunction } from '../function/index.mjs';
import { type TTuple } from '../tuple/index.mjs';
import { type TNever } from '../never/index.mjs';
export type TParameters<Type extends TSchema> = (Type extends TFunction<infer Parameters extends TSchema[], infer _ReturnType extends TSchema> ? TTuple<Parameters> : TNever);
/** `[JavaScript]` Extracts the Parameters from the given Function type */
export declare function Parameters<Type extends TSchema>(schema: Type, options?: SchemaOptions): TParameters<Type>;
