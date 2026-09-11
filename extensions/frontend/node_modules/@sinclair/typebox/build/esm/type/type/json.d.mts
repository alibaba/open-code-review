import { type TAny } from '../any/index.mjs';
import { type TArray, type ArrayOptions } from '../array/index.mjs';
import { type TBoolean } from '../boolean/index.mjs';
import { type TComposite } from '../composite/index.mjs';
import { type TConst } from '../const/index.mjs';
import { type TEnum, type TEnumKey, type TEnumValue } from '../enum/index.mjs';
import { type TExclude, type TExcludeFromMappedResult, type TExcludeFromTemplateLiteral } from '../exclude/index.mjs';
import { type TExtends, type TExtendsFromMappedKey, type TExtendsFromMappedResult } from '../extends/index.mjs';
import { type TExtract, type TExtractFromMappedResult, type TExtractFromTemplateLiteral } from '../extract/index.mjs';
import { TIndex, type TIndexPropertyKeys, type TIndexFromMappedKey, type TIndexFromMappedResult, type TIndexFromComputed } from '../indexed/index.mjs';
import { type IntegerOptions, type TInteger } from '../integer/index.mjs';
import { Intersect, type IntersectOptions } from '../intersect/index.mjs';
import { type TCapitalize, type TUncapitalize, type TLowercase, type TUppercase } from '../intrinsic/index.mjs';
import { type TKeyOf } from '../keyof/index.mjs';
import { type TLiteral, type TLiteralValue } from '../literal/index.mjs';
import { type TMappedFunction, type TMapped, type TMappedResult } from '../mapped/index.mjs';
import { type TNever } from '../never/index.mjs';
import { type TNot } from '../not/index.mjs';
import { type TNull } from '../null/index.mjs';
import { type TMappedKey } from '../mapped/index.mjs';
import { TModule } from '../module/index.mjs';
import { type TNumber, type NumberOptions } from '../number/index.mjs';
import { type TObject, type TProperties, type ObjectOptions } from '../object/index.mjs';
import { type TOmit } from '../omit/index.mjs';
import { type TOptionalWithFlag, type TOptionalFromMappedResult } from '../optional/index.mjs';
import { type TPartial, type TPartialFromMappedResult } from '../partial/index.mjs';
import { type TPick } from '../pick/index.mjs';
import { type TReadonlyWithFlag, type TReadonlyFromMappedResult } from '../readonly/index.mjs';
import { type TReadonlyOptional } from '../readonly-optional/index.mjs';
import { type TRecordOrObject } from '../record/index.mjs';
import { type TRecursive, type TThis } from '../recursive/index.mjs';
import { type TRef, type TRefUnsafe } from '../ref/index.mjs';
import { type TRequired, type TRequiredFromMappedResult } from '../required/index.mjs';
import { type TRest } from '../rest/index.mjs';
import { type TSchema, type SchemaOptions } from '../schema/index.mjs';
import { type TString, type StringOptions } from '../string/index.mjs';
import { type TTemplateLiteral, type TTemplateLiteralKind, type TTemplateLiteralSyntax } from '../template-literal/index.mjs';
import { TransformDecodeBuilder } from '../transform/index.mjs';
import { type TTuple } from '../tuple/index.mjs';
import { Union } from '../union/index.mjs';
import { type TUnknown } from '../unknown/index.mjs';
import { type TUnsafe, type UnsafeOptions } from '../unsafe/index.mjs';
/** Json Type Builder with Static Resolution for TypeScript */
export declare class JsonTypeBuilder {
    /** `[Json]` Creates a Readonly and Optional property */
    ReadonlyOptional<Type extends TSchema>(type: Type): TReadonlyOptional<Type>;
    /** `[Json]` Creates a Readonly property */
    Readonly<Type extends TMappedResult, Flag extends boolean>(type: Type, enable: Flag): TReadonlyFromMappedResult<Type, Flag>;
    /** `[Json]` Creates a Readonly property */
    Readonly<Type extends TSchema, Flag extends boolean>(type: Type, enable: Flag): TReadonlyWithFlag<Type, Flag>;
    /** `[Json]` Creates a Optional property */
    Readonly<Type extends TMappedResult>(type: Type): TReadonlyFromMappedResult<Type, true>;
    /** `[Json]` Creates a Readonly property */
    Readonly<Type extends TSchema>(type: Type): TReadonlyWithFlag<Type, true>;
    /** `[Json]` Creates a Optional property */
    Optional<Type extends TMappedResult, Flag extends boolean>(type: Type, enable: Flag): TOptionalFromMappedResult<Type, Flag>;
    /** `[Json]` Creates a Optional property */
    Optional<Type extends TSchema, Flag extends boolean>(type: Type, enable: Flag): TOptionalWithFlag<Type, Flag>;
    /** `[Json]` Creates a Optional property */
    Optional<Type extends TMappedResult>(type: Type): TOptionalFromMappedResult<Type, true>;
    /** `[Json]` Creates a Optional property */
    Optional<Type extends TSchema>(type: Type): TOptionalWithFlag<Type, true>;
    /** `[Json]` Creates an Any type */
    Any(options?: SchemaOptions): TAny;
    /** `[Json]` Creates an Array type */
    Array<Type extends TSchema>(items: Type, options?: ArrayOptions): TArray<Type>;
    /** `[Json]` Creates a Boolean type */
    Boolean(options?: SchemaOptions): TBoolean;
    /** `[Json]` Intrinsic function to Capitalize LiteralString types */
    Capitalize<T extends TSchema>(schema: T, options?: SchemaOptions): TCapitalize<T>;
    /** `[Json]` Creates a Composite object type */
    Composite<T extends TSchema[]>(schemas: [...T], options?: ObjectOptions): TComposite<T>;
    /** `[JavaScript]` Creates a readonly const type from the given value. */
    Const</* const (not supported in 4.0) */ T>(value: T, options?: SchemaOptions): TConst<T>;
    /** `[Json]` Creates a Enum type */
    Enum<V extends TEnumValue, T extends Record<TEnumKey, V>>(item: T, options?: SchemaOptions): TEnum<T>;
    /** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
    Exclude<L extends TMappedResult, R extends TSchema>(unionType: L, excludedMembers: R, options?: SchemaOptions): TExcludeFromMappedResult<L, R>;
    /** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
    Exclude<L extends TTemplateLiteral, R extends TSchema>(unionType: L, excludedMembers: R, options?: SchemaOptions): TExcludeFromTemplateLiteral<L, R>;
    /** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
    Exclude<L extends TSchema, R extends TSchema>(unionType: L, excludedMembers: R, options?: SchemaOptions): TExclude<L, R>;
    /** `[Json]` Creates a Conditional type */
    Extends<L extends TMappedResult, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtendsFromMappedResult<L, R, T, F>;
    /** `[Json]` Creates a Conditional type */
    Extends<L extends TMappedKey, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtendsFromMappedKey<L, R, T, F>;
    /** `[Json]` Creates a Conditional type */
    Extends<L extends TSchema, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtends<L, R, T, F>;
    /** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
    Extract<L extends TMappedResult, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtractFromMappedResult<L, R>;
    /** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
    Extract<L extends TTemplateLiteral, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtractFromTemplateLiteral<L, R>;
    /** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
    Extract<L extends TSchema, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtract<L, R>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TRef, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TSchema, Key extends TRef>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TRef, Key extends TRef>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TSchema, MappedResult extends TMappedResult>(type: Type, mappedResult: MappedResult, options?: SchemaOptions): TIndexFromMappedResult<Type, MappedResult>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TSchema, MappedKey extends TMappedKey>(type: Type, mappedKey: MappedKey, options?: SchemaOptions): TIndexFromMappedKey<Type, MappedKey>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TSchema, Key extends TSchema, PropertyKeys extends PropertyKey[] = TIndexPropertyKeys<Key>>(T: Type, K: Key, options?: SchemaOptions): TIndex<Type, PropertyKeys>;
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index<Type extends TSchema, PropertyKeys extends PropertyKey[]>(type: Type, propertyKeys: readonly [...PropertyKeys], options?: SchemaOptions): TIndex<Type, PropertyKeys>;
    /** `[Json]` Creates an Integer type */
    Integer(options?: IntegerOptions): TInteger;
    /** `[Json]` Creates an Intersect type */
    Intersect<Types extends TSchema[]>(types: [...Types], options?: IntersectOptions): Intersect<Types>;
    /** `[Json]` Creates a KeyOf type */
    KeyOf<Type extends TSchema>(type: Type, options?: SchemaOptions): TKeyOf<Type>;
    /** `[Json]` Creates a Literal type */
    Literal<LiteralValue extends TLiteralValue>(literalValue: LiteralValue, options?: SchemaOptions): TLiteral<LiteralValue>;
    /** `[Json]` Intrinsic function to Lowercase LiteralString types */
    Lowercase<Type extends TSchema>(type: Type, options?: SchemaOptions): TLowercase<Type>;
    /** `[Json]` Creates a Mapped object type */
    Mapped<K extends TSchema, I extends PropertyKey[] = TIndexPropertyKeys<K>, F extends TMappedFunction<I> = TMappedFunction<I>, R extends TMapped<I, F> = TMapped<I, F>>(key: K, map: F, options?: ObjectOptions): R;
    /** `[Json]` Creates a Mapped object type */
    Mapped<K extends PropertyKey[], F extends TMappedFunction<K> = TMappedFunction<K>, R extends TMapped<K, F> = TMapped<K, F>>(key: [...K], map: F, options?: ObjectOptions): R;
    /** `[Json]` Creates a Type Definition Module. */
    Module<Properties extends TProperties>(properties: Properties): TModule<Properties>;
    /** `[Json]` Creates a Never type */
    Never(options?: SchemaOptions): TNever;
    /** `[Json]` Creates a Not type */
    Not<T extends TSchema>(type: T, options?: SchemaOptions): TNot<T>;
    /** `[Json]` Creates a Null type */
    Null(options?: SchemaOptions): TNull;
    /** `[Json]` Creates a Number type */
    Number(options?: NumberOptions): TNumber;
    /** `[Json]` Creates an Object type */
    Object<T extends TProperties>(properties: T, options?: ObjectOptions): TObject<T>;
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Omit<Type extends TSchema, Key extends PropertyKey[]>(type: Type, key: readonly [...Key], options?: SchemaOptions): TOmit<Type, Key>;
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Omit<Type extends TSchema, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TOmit<Type, Key>;
    /** `[Json]` Constructs a type where all properties are optional */
    Partial<MappedResult extends TMappedResult>(type: MappedResult, options?: SchemaOptions): TPartialFromMappedResult<MappedResult>;
    /** `[Json]` Constructs a type where all properties are optional */
    Partial<Type extends TSchema>(type: Type, options?: SchemaOptions): TPartial<Type>;
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Pick<Type extends TSchema, Key extends PropertyKey[]>(type: Type, key: readonly [...Key], options?: SchemaOptions): TPick<Type, Key>;
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Pick<Type extends TSchema, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TPick<Type, Key>;
    /** `[Json]` Creates a Record type */
    Record<Key extends TSchema, Value extends TSchema>(key: Key, value: Value, options?: ObjectOptions): TRecordOrObject<Key, Value>;
    /** `[Json]` Creates a Recursive type */
    Recursive<T extends TSchema>(callback: (thisType: TThis) => T, options?: SchemaOptions): TRecursive<T>;
    /** `[Json]` Creates a Ref type.*/
    Ref<Ref extends string>($ref: Ref, options?: SchemaOptions): TRef<Ref>;
    /**
     * @deprecated `[Json]` Creates a Ref type. This signature was deprecated in 0.34.0 where Ref requires callers to pass
     * a `string` value for the reference (and not a schema).
     *
     * To adhere to the 0.34.0 signature, Ref implementations should be updated to the following.
     *
     * ```typescript
     * // pre-0.34.0
     *
     * const T = Type.String({ $id: 'T' })
     *
     * const R = Type.Ref(T)
     * ```
     * should be changed to the following
     *
     * ```typescript
     * // post-0.34.0
     *
     * const T = Type.String({ $id: 'T' })
     *
     * const R = Type.Unsafe<Static<typeof T>>(Type.Ref('T'))
     * ```
     * You can also create a generic function to replicate the pre-0.34.0 signature if required
     *
     * ```typescript
     * const LegacyRef = <T extends TSchema>(schema: T) => Type.Unsafe<Static<T>>(Type.Ref(schema.$id!))
     * ```
     */
    Ref<Type extends TSchema>(type: Type, options?: SchemaOptions): TRefUnsafe<Type>;
    /** `[Json]` Constructs a type where all properties are required */
    Required<MappedResult extends TMappedResult>(type: MappedResult, options?: SchemaOptions): TRequiredFromMappedResult<MappedResult>;
    /** `[Json]` Constructs a type where all properties are required */
    Required<Type extends TSchema>(type: Type, options?: SchemaOptions): TRequired<Type>;
    /** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
    Rest<Type extends TSchema>(type: Type): TRest<Type>;
    /** `[Json]` Creates a String type */
    String(options?: StringOptions): TString;
    /** `[Json]` Creates a TemplateLiteral type from template dsl string */
    TemplateLiteral<Syntax extends string>(syntax: Syntax, options?: SchemaOptions): TTemplateLiteralSyntax<Syntax>;
    /** `[Json]` Creates a TemplateLiteral type */
    TemplateLiteral<Kinds extends TTemplateLiteralKind[]>(kinds: [...Kinds], options?: SchemaOptions): TTemplateLiteral<Kinds>;
    /** `[Json]` Creates a Transform type */
    Transform<Type extends TSchema>(type: Type): TransformDecodeBuilder<Type>;
    /** `[Json]` Creates a Tuple type */
    Tuple<Types extends TSchema[]>(types: [...Types], options?: SchemaOptions): TTuple<Types>;
    /** `[Json]` Intrinsic function to Uncapitalize LiteralString types */
    Uncapitalize<Type extends TSchema>(type: Type, options?: SchemaOptions): TUncapitalize<Type>;
    /** `[Json]` Creates a Union type */
    Union<Types extends TSchema[]>(types: [...Types], options?: SchemaOptions): Union<Types>;
    /** `[Json]` Creates an Unknown type */
    Unknown(options?: SchemaOptions): TUnknown;
    /** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
    Unsafe<T>(options?: UnsafeOptions): TUnsafe<T>;
    /** `[Json]` Intrinsic function to Uppercase LiteralString types */
    Uppercase<T extends TSchema>(schema: T, options?: SchemaOptions): TUppercase<T>;
}
