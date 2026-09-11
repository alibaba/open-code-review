import type { SchemaOptions } from '../schema/index';
import type { Ensure, Evaluate } from '../helpers/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TPick } from './pick';
type TFromProperties<Properties extends TProperties, PropertyKeys extends PropertyKey[]> = ({
    [K2 in keyof Properties]: TPick<Properties[K2], PropertyKeys>;
});
type TFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[]> = (Evaluate<TFromProperties<MappedResult['properties'], PropertyKeys>>);
export type TPickFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[], Properties extends TProperties = TFromMappedResult<MappedResult, PropertyKeys>> = (Ensure<TMappedResult<Properties>>);
export declare function PickFromMappedResult<MappedResult extends TMappedResult, PropertyKeys extends PropertyKey[], Properties extends TProperties = TFromMappedResult<MappedResult, PropertyKeys>>(mappedResult: MappedResult, propertyKeys: [...PropertyKeys], options?: SchemaOptions): TMappedResult<Properties>;
export {};
