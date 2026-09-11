import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
import { TUnsafe } from '../unsafe/index';
import { Static } from '../static/index';
export interface TRef<Ref extends string = string> extends TSchema {
    [Kind]: 'Ref';
    static: unknown;
    $ref: Ref;
}
export type TRefUnsafe<Type extends TSchema> = TUnsafe<Static<Type>>;
/** `[Json]` Creates a Ref type.*/
export declare function Ref<Ref extends string>($ref: Ref, options?: SchemaOptions): TRef<Ref>;
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
export declare function Ref<Type extends TSchema>(type: Type, options?: SchemaOptions): TRefUnsafe<Type>;
