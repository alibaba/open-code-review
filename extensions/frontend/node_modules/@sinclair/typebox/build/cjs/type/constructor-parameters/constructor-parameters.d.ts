import type { TSchema, SchemaOptions } from '../schema/index';
import type { TConstructor } from '../constructor/index';
import { type TTuple } from '../tuple/index';
import { type TNever } from '../never/index';
export type TConstructorParameters<Type extends TSchema> = (Type extends TConstructor<infer Parameters extends TSchema[], infer _InstanceType extends TSchema> ? TTuple<Parameters> : TNever);
/** `[JavaScript]` Extracts the ConstructorParameters from the given Constructor type */
export declare function ConstructorParameters<Type extends TSchema>(schema: Type, options?: SchemaOptions): TConstructorParameters<Type>;
