import { ValueErrorIterator } from '../errors/index';
import { TypeBoxError } from '../type/error/index';
import type { TSchema } from '../type/schema/index';
import type { Static, StaticDecode, StaticEncode } from '../type/static/index';
export type CheckFunction = (value: unknown) => boolean;
export declare class TypeCheck<T extends TSchema> {
    private readonly schema;
    private readonly references;
    private readonly checkFunc;
    private readonly code;
    private readonly hasTransform;
    constructor(schema: T, references: TSchema[], checkFunc: CheckFunction, code: string);
    /** Returns the generated assertion code used to validate this type. */
    Code(): string;
    /** Returns the schema type used to validate */
    Schema(): T;
    /** Returns reference types used to validate */
    References(): TSchema[];
    /** Returns an iterator for each error in this value. */
    Errors(value: unknown): ValueErrorIterator;
    /** Returns true if the value matches the compiled type. */
    Check(value: unknown): value is Static<T>;
    /** Decodes a value or throws if error */
    Decode<Static = StaticDecode<T>, Result extends Static = Static>(value: unknown): Result;
    /** Encodes a value or throws if error */
    Encode<Static = StaticEncode<T>, Result extends Static = Static>(value: unknown): Result;
}
export declare class TypeCompilerUnknownTypeError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema);
}
export declare class TypeCompilerTypeGuardError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema);
}
export declare namespace Policy {
    function IsExactOptionalProperty(value: string, key: string, expression: string): string;
    function IsObjectLike(value: string): string;
    function IsRecordLike(value: string): string;
    function IsNumberLike(value: string): string;
    function IsVoidLike(value: string): string;
}
export type TypeCompilerLanguageOption = 'typescript' | 'javascript';
export interface TypeCompilerCodegenOptions {
    language?: TypeCompilerLanguageOption;
}
/** Compiles Types for Runtime Type Checking */
export declare namespace TypeCompiler {
    /** Generates the code used to assert this type and returns it as a string */
    function Code<T extends TSchema>(schema: T, references: TSchema[], options?: TypeCompilerCodegenOptions): string;
    /** Generates the code used to assert this type and returns it as a string */
    function Code<T extends TSchema>(schema: T, options?: TypeCompilerCodegenOptions): string;
    /** Compiles a TypeBox type for optimal runtime type checking. Types must be valid TypeBox types of TSchema */
    function Compile<T extends TSchema>(schema: T, references?: TSchema[]): TypeCheck<T>;
}
