import { Kind } from '../symbols/index';
import type { TSchema } from '../schema/index';
import type { Static } from '../static/index';
import type { Evaluate, Ensure, Assert } from '../helpers/index';
import { type TAny } from '../any/index';
import { type TBoolean } from '../boolean/index';
import { type TEnumRecord, type TEnum } from '../enum/index';
import { type TInteger } from '../integer/index';
import { type TLiteral, type TLiteralValue } from '../literal/index';
import { type TNever } from '../never/index';
import { type TNumber } from '../number/index';
import { type TObject, type TProperties, type TAdditionalProperties, type ObjectOptions } from '../object/index';
import { type TRegExp } from '../regexp/index';
import { type TString } from '../string/index';
import { type TUnion } from '../union/index';
import { TIsTemplateLiteralFinite, type TTemplateLiteral } from '../template-literal/index';
type TFromTemplateLiteralKeyInfinite<Key extends TTemplateLiteral, Type extends TSchema> = Ensure<TRecord<Key, Type>>;
type TFromTemplateLiteralKeyFinite<Key extends TTemplateLiteral, Type extends TSchema, I extends string = Static<Key>> = (Ensure<TObject<Evaluate<{
    [_ in I]: Type;
}>>>);
type TFromTemplateLiteralKey<Key extends TTemplateLiteral, Type extends TSchema> = TIsTemplateLiteralFinite<Key> extends false ? TFromTemplateLiteralKeyInfinite<Key, Type> : TFromTemplateLiteralKeyFinite<Key, Type>;
type TFromEnumKey<Key extends Record<string, string | number>, Type extends TSchema> = Ensure<TObject<{
    [_ in Key[keyof Key]]: Type;
}>>;
type TFromUnionKeyLiteralString<Key extends TLiteral<string>, Type extends TSchema> = {
    [_ in Key['const']]: Type;
};
type TFromUnionKeyLiteralNumber<Key extends TLiteral<number>, Type extends TSchema> = {
    [_ in Key['const']]: Type;
};
type TFromUnionKeyVariants<Keys extends TSchema[], Type extends TSchema, Result extends TProperties = {}> = Keys extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? (Left extends TUnion<infer Types extends TSchema[]> ? TFromUnionKeyVariants<Right, Type, Result & TFromUnionKeyVariants<Types, Type>> : Left extends TLiteral<string> ? TFromUnionKeyVariants<Right, Type, Result & TFromUnionKeyLiteralString<Left, Type>> : Left extends TLiteral<number> ? TFromUnionKeyVariants<Right, Type, Result & TFromUnionKeyLiteralNumber<Left, Type>> : {}) : Result;
type TFromUnionKey<Key extends TSchema[], Type extends TSchema, Properties extends TProperties = TFromUnionKeyVariants<Key, Type>> = (Ensure<TObject<Evaluate<Properties>>>);
type TFromLiteralKey<Key extends TLiteralValue, Type extends TSchema> = (Ensure<TObject<{
    [_ in Assert<Key, PropertyKey>]: Type;
}>>);
type TFromRegExpKey<_Key extends TRegExp, Type extends TSchema> = (Ensure<TRecord<TRegExp, Type>>);
type TFromStringKey<_Key extends TString, Type extends TSchema> = (Ensure<TRecord<TString, Type>>);
type TFromAnyKey<_Key extends TAny, Type extends TSchema> = (Ensure<TRecord<TAny, Type>>);
type TFromNeverKey<_Key extends TNever, Type extends TSchema> = (Ensure<TRecord<TNever, Type>>);
type TFromBooleanKey<_Key extends TBoolean, Type extends TSchema> = (Ensure<TObject<{
    true: Type;
    false: Type;
}>>);
type TFromIntegerKey<_Key extends TSchema, Type extends TSchema> = (Ensure<TRecord<TNumber, Type>>);
type TFromNumberKey<_Key extends TSchema, Type extends TSchema> = (Ensure<TRecord<TNumber, Type>>);
type RecordStatic<Key extends TSchema, Type extends TSchema, P extends unknown[]> = (Evaluate<{
    [_ in Assert<Static<Key>, PropertyKey>]: Static<Type, P>;
}>);
export interface TRecord<Key extends TSchema = TSchema, Type extends TSchema = TSchema> extends TSchema {
    [Kind]: 'Record';
    static: RecordStatic<Key, Type, this['params']>;
    type: 'object';
    patternProperties: {
        [pattern: string]: Type;
    };
    additionalProperties: TAdditionalProperties;
}
export type TRecordOrObject<Key extends TSchema, Type extends TSchema> = (Key extends TTemplateLiteral ? TFromTemplateLiteralKey<Key, Type> : Key extends TEnum<infer Enum extends TEnumRecord> ? TFromEnumKey<Enum, Type> : Key extends TUnion<infer Types extends TSchema[]> ? TFromUnionKey<Types, Type> : Key extends TLiteral<infer Value extends TLiteralValue> ? TFromLiteralKey<Value, Type> : Key extends TBoolean ? TFromBooleanKey<Key, Type> : Key extends TInteger ? TFromIntegerKey<Key, Type> : Key extends TNumber ? TFromNumberKey<Key, Type> : Key extends TRegExp ? TFromRegExpKey<Key, Type> : Key extends TString ? TFromStringKey<Key, Type> : Key extends TAny ? TFromAnyKey<Key, Type> : Key extends TNever ? TFromNeverKey<Key, Type> : TNever);
/** `[Json]` Creates a Record type */
export declare function Record<Key extends TSchema, Type extends TSchema>(key: Key, type: Type, options?: ObjectOptions): TRecordOrObject<Key, Type>;
/** Gets the Records Pattern */
export declare function RecordPattern(record: TRecord): string;
/** Gets the Records Key Type */
export type TRecordKey<Type extends TRecord, Result extends TSchema = Type extends TRecord<infer Key extends TSchema, TSchema> ? (Key extends TNumber ? TNumber : Key extends TString ? TString : TString) : TString> = Result;
/** Gets the Records Key Type */
export declare function RecordKey<Type extends TRecord>(type: Type): TRecordKey<Type>;
/** Gets a Record Value Type */
export type TRecordValue<Type extends TRecord, Result extends TSchema = (Type extends TRecord<TSchema, infer Value extends TSchema> ? Value : TNever)> = Result;
/** Gets a Record Value Type */
export declare function RecordValue<Type extends TRecord>(type: Type): TRecordValue<Type>;
export {};
