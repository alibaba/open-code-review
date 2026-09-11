import type { TSchema } from '../schema/index';
import type { Evaluate } from '../helpers/index';
import { type TIntersectEvaluated } from '../intersect/index';
import { type TIndexFromPropertyKeys } from '../indexed/index';
import { type TKeyOfPropertyKeys } from '../keyof/index';
import { type TNever } from '../never/index';
import { type TObject, type TProperties, type ObjectOptions } from '../object/index';
import { TSetDistinct } from '../sets/index';
type TCompositeKeys<T extends TSchema[], Acc extends PropertyKey[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TCompositeKeys<R, [...Acc, ...TKeyOfPropertyKeys<L>]> : TSetDistinct<Acc>);
type TFilterNever<T extends TSchema[], Acc extends TSchema[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? L extends TNever ? TFilterNever<R, [...Acc]> : TFilterNever<R, [...Acc, L]> : Acc);
type TCompositeProperty<T extends TSchema[], K extends PropertyKey, Acc extends TSchema[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TCompositeProperty<R, K, [...Acc, ...TIndexFromPropertyKeys<L, [K]>]> : TFilterNever<Acc>);
type TCompositeProperties<T extends TSchema[], K extends PropertyKey[], Acc = {}> = (K extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TCompositeProperties<T, R, Acc & {
    [_ in L]: TIntersectEvaluated<TCompositeProperty<T, L>>;
}> : Acc);
type TCompositeEvaluate<T extends TSchema[], K extends PropertyKey[] = TCompositeKeys<T>, P extends TProperties = Evaluate<TCompositeProperties<T, K>>, R extends TSchema = TObject<P>> = R;
export type TComposite<T extends TSchema[]> = TCompositeEvaluate<T>;
export declare function Composite<T extends TSchema[]>(T: [...T], options?: ObjectOptions): TComposite<T>;
export {};
