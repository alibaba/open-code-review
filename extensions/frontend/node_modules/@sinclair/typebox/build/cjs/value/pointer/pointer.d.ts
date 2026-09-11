import { TypeBoxError } from '../../type/error/index';
export declare class ValuePointerRootSetError extends TypeBoxError {
    readonly value: unknown;
    readonly path: string;
    readonly update: unknown;
    constructor(value: unknown, path: string, update: unknown);
}
export declare class ValuePointerRootDeleteError extends TypeBoxError {
    readonly value: unknown;
    readonly path: string;
    constructor(value: unknown, path: string);
}
/** Formats the given pointer into navigable key components */
export declare function Format(pointer: string): IterableIterator<string>;
/** Sets the value at the given pointer. If the value at the pointer does not exist it is created */
export declare function Set(value: any, pointer: string, update: unknown): void;
/** Deletes a value at the given pointer */
export declare function Delete(value: any, pointer: string): void;
/** Returns true if a value exists at the given pointer */
export declare function Has(value: any, pointer: string): boolean;
/** Gets the value at the given pointer */
export declare function Get(value: any, pointer: string): any;
