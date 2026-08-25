import { type TSchema, type SchemaOptions } from '../schema/index.mjs';
import { type TFunction } from '../function/index.mjs';
import { type TNever } from '../never/index.mjs';
export type TReturnType<Type extends TSchema, Result extends TSchema = Type extends TFunction<infer _Parameters extends TSchema[], infer ReturnType extends TSchema> ? ReturnType : TNever> = Result;
/** `[JavaScript]` Extracts the ReturnType from the given Function type */
export declare function ReturnType<Type extends TSchema>(schema: Type, options?: SchemaOptions): TReturnType<Type>;
