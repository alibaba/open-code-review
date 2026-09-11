import type { SchemaOptions } from '../schema/index';
import type { Ensure, Evaluate } from '../helpers/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TOmit } from './omit';
type TFromProperties<Properties extends TProperties, PropertyKeys extends PropertyKey[]> = ({
    [K2 in keyof Properties]: TOmit<Properties[K2], PropertyKeys>;
});
type TFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[]> = (Evaluate<TFromProperties<MappedResult['properties'], PropertyKeys>>);
export type TOmitFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[], Properties extends TProperties = TFromMappedResult<MappedResult, PropertyKeys>> = (Ensure<TMappedResult<Properties>>);
export declare function OmitFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[], Properties extends TProperties = TFromMappedResult<MappedResult, PropertyKeys>>(mappedResult: MappedResult, propertyKeys: [...PropertyKeys], options?: SchemaOptions): TMappedResult<Properties>;
export {};
