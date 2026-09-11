import { type TSchema, SchemaOptions } from '../schema/index';
import { type TConstructor } from '../constructor/index';
import { type TNever } from '../never/index';
export type TInstanceType<Type extends TSchema, Result extends TSchema = Type extends TConstructor<infer _Parameters extends TSchema[], infer InstanceType extends TSchema> ? InstanceType : TNever> = Result;
/** `[JavaScript]` Extracts the InstanceType from the given Constructor type */
export declare function InstanceType<Type extends TSchema>(schema: Type, options?: SchemaOptions): TInstanceType<Type>;
