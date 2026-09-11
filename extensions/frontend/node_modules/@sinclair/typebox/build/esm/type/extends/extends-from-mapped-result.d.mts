import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TExtends } from './extends.mjs';
type TFromProperties<P extends TProperties, Right extends TSchema, False extends TSchema, True extends TSchema> = ({
    [K2 in keyof P]: TExtends<P[K2], Right, False, True>;
});
type TFromMappedResult<Left extends TMappedResult, Right extends TSchema, True extends TSchema, False extends TSchema> = (TFromProperties<Left['properties'], Right, True, False>);
export type TExtendsFromMappedResult<Left extends TMappedResult, Right extends TSchema, True extends TSchema, False extends TSchema, P extends TProperties = TFromMappedResult<Left, Right, True, False>> = (TMappedResult<P>);
export declare function ExtendsFromMappedResult<Left extends TMappedResult, Right extends TSchema, True extends TSchema, False extends TSchema, P extends TProperties = TFromMappedResult<Left, Right, True, False>>(Left: Left, Right: Right, True: True, False: False, options?: SchemaOptions): TMappedResult<P>;
export {};
