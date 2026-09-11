import { type TUnsafe } from '../type/unsafe/index.mjs';
import { TypeBoxError } from '../type/error/index.mjs';
export declare class TypeSystemDuplicateTypeKind extends TypeBoxError {
    constructor(kind: string);
}
export declare class TypeSystemDuplicateFormat extends TypeBoxError {
    constructor(kind: string);
}
export type TypeFactoryFunction<Type, Options = Record<PropertyKey, unknown>> = (options?: Partial<Options>) => TUnsafe<Type>;
/** Creates user defined types and formats and provides overrides for value checking behaviours */
export declare namespace TypeSystem {
    /** Creates a new type */
    function Type<Type, Options = Record<PropertyKey, unknown>>(kind: string, check: (options: Options, value: unknown) => boolean): TypeFactoryFunction<Type, Options>;
    /** Creates a new string format */
    function Format<F extends string>(format: F, check: (value: string) => boolean): F;
}
