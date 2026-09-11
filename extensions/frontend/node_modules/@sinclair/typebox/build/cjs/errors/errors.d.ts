import { TypeBoxError } from '../type/error/index';
import type { TSchema } from '../type/schema/index';
export declare enum ValueErrorType {
    ArrayContains = 0,
    ArrayMaxContains = 1,
    ArrayMaxItems = 2,
    ArrayMinContains = 3,
    ArrayMinItems = 4,
    ArrayUniqueItems = 5,
    Array = 6,
    AsyncIterator = 7,
    BigIntExclusiveMaximum = 8,
    BigIntExclusiveMinimum = 9,
    BigIntMaximum = 10,
    BigIntMinimum = 11,
    BigIntMultipleOf = 12,
    BigInt = 13,
    Boolean = 14,
    DateExclusiveMaximumTimestamp = 15,
    DateExclusiveMinimumTimestamp = 16,
    DateMaximumTimestamp = 17,
    DateMinimumTimestamp = 18,
    DateMultipleOfTimestamp = 19,
    Date = 20,
    Function = 21,
    IntegerExclusiveMaximum = 22,
    IntegerExclusiveMinimum = 23,
    IntegerMaximum = 24,
    IntegerMinimum = 25,
    IntegerMultipleOf = 26,
    Integer = 27,
    IntersectUnevaluatedProperties = 28,
    Intersect = 29,
    Iterator = 30,
    Kind = 31,
    Literal = 32,
    Never = 33,
    Not = 34,
    Null = 35,
    NumberExclusiveMaximum = 36,
    NumberExclusiveMinimum = 37,
    NumberMaximum = 38,
    NumberMinimum = 39,
    NumberMultipleOf = 40,
    Number = 41,
    ObjectAdditionalProperties = 42,
    ObjectMaxProperties = 43,
    ObjectMinProperties = 44,
    ObjectRequiredProperty = 45,
    Object = 46,
    Promise = 47,
    RegExp = 48,
    StringFormatUnknown = 49,
    StringFormat = 50,
    StringMaxLength = 51,
    StringMinLength = 52,
    StringPattern = 53,
    String = 54,
    Symbol = 55,
    TupleLength = 56,
    Tuple = 57,
    Uint8ArrayMaxByteLength = 58,
    Uint8ArrayMinByteLength = 59,
    Uint8Array = 60,
    Undefined = 61,
    Union = 62,
    Void = 63
}
export interface ValueError {
    type: ValueErrorType;
    schema: TSchema;
    path: string;
    value: unknown;
    message: string;
    errors: ValueErrorIterator[];
}
export declare class ValueErrorsUnknownTypeError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema);
}
export declare class ValueErrorIterator {
    private readonly iterator;
    constructor(iterator: IterableIterator<ValueError>);
    [Symbol.iterator](): IterableIterator<ValueError>;
    /** Returns the first value error or undefined if no errors */
    First(): ValueError | undefined;
}
/** Returns an iterator for each error in this value. */
export declare function Errors<T extends TSchema>(schema: T, references: TSchema[], value: unknown): ValueErrorIterator;
/** Returns an iterator for each error in this value. */
export declare function Errors<T extends TSchema>(schema: T, value: unknown): ValueErrorIterator;
