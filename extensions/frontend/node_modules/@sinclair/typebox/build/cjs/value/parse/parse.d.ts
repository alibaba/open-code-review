import { TypeBoxError } from '../../type/error/index';
import { TSchema } from '../../type/schema/index';
import { StaticDecode } from '../../type/static/index';
export declare class ParseError extends TypeBoxError {
    constructor(message: string);
}
export type TParseOperation = 'Assert' | 'Cast' | 'Clean' | 'Clone' | 'Convert' | 'Decode' | 'Default' | 'Encode' | ({} & string);
export type TParseFunction = (type: TSchema, references: TSchema[], value: unknown) => unknown;
export declare namespace ParseRegistry {
    function Delete(key: string): void;
    function Set(key: string, callback: TParseFunction): void;
    function Get(key: string): TParseFunction | undefined;
}
export declare const ParseDefault: readonly ['Clone', 'Clean', 'Default', 'Convert', 'Assert', 'Decode'];
/** Parses a value using the default parse pipeline. Will throws an `AssertError` if invalid. */
export declare function Parse<Type extends TSchema, Output = StaticDecode<Type>, Result extends Output = Output>(schema: Type, references: TSchema[], value: unknown): Result;
/** Parses a value using the default parse pipeline. Will throws an `AssertError` if invalid. */
export declare function Parse<Type extends TSchema, Output = StaticDecode<Type>, Result extends Output = Output>(schema: Type, value: unknown): Result;
/** Parses a value using the specified operations. */
export declare function Parse<Type extends TSchema>(operations: TParseOperation[], schema: Type, references: TSchema[], value: unknown): unknown;
/** Parses a value using the specified operations. */
export declare function Parse<Type extends TSchema>(operations: TParseOperation[], schema: Type, value: unknown): unknown;
