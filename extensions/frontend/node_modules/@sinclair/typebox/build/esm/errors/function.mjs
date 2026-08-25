import { Kind } from '../type/symbols/index.mjs';
import { ValueErrorType } from './errors.mjs';
/** Creates an error message using en-US as the default locale */
export function DefaultErrorFunction(error) {
    switch (error.errorType) {
        case ValueErrorType.ArrayContains:
            return 'Expected array to contain at least one matching value';
        case ValueErrorType.ArrayMaxContains:
            return `Expected array to contain no more than ${error.schema.maxContains} matching values`;
        case ValueErrorType.ArrayMinContains:
            return `Expected array to contain at least ${error.schema.minContains} matching values`;
        case ValueErrorType.ArrayMaxItems:
            return `Expected array length to be less or equal to ${error.schema.maxItems}`;
        case ValueErrorType.ArrayMinItems:
            return `Expected array length to be greater or equal to ${error.schema.minItems}`;
        case ValueErrorType.ArrayUniqueItems:
            return 'Expected array elements to be unique';
        case ValueErrorType.Array:
            return 'Expected array';
        case ValueErrorType.AsyncIterator:
            return 'Expected AsyncIterator';
        case ValueErrorType.BigIntExclusiveMaximum:
            return `Expected bigint to be less than ${error.schema.exclusiveMaximum}`;
        case ValueErrorType.BigIntExclusiveMinimum:
            return `Expected bigint to be greater than ${error.schema.exclusiveMinimum}`;
        case ValueErrorType.BigIntMaximum:
            return `Expected bigint to be less or equal to ${error.schema.maximum}`;
        case ValueErrorType.BigIntMinimum:
            return `Expected bigint to be greater or equal to ${error.schema.minimum}`;
        case ValueErrorType.BigIntMultipleOf:
            return `Expected bigint to be a multiple of ${error.schema.multipleOf}`;
        case ValueErrorType.BigInt:
            return 'Expected bigint';
        case ValueErrorType.Boolean:
            return 'Expected boolean';
        case ValueErrorType.DateExclusiveMinimumTimestamp:
            return `Expected Date timestamp to be greater than ${error.schema.exclusiveMinimumTimestamp}`;
        case ValueErrorType.DateExclusiveMaximumTimestamp:
            return `Expected Date timestamp to be less than ${error.schema.exclusiveMaximumTimestamp}`;
        case ValueErrorType.DateMinimumTimestamp:
            return `Expected Date timestamp to be greater or equal to ${error.schema.minimumTimestamp}`;
        case ValueErrorType.DateMaximumTimestamp:
            return `Expected Date timestamp to be less or equal to ${error.schema.maximumTimestamp}`;
        case ValueErrorType.DateMultipleOfTimestamp:
            return `Expected Date timestamp to be a multiple of ${error.schema.multipleOfTimestamp}`;
        case ValueErrorType.Date:
            return 'Expected Date';
        case ValueErrorType.Function:
            return 'Expected function';
        case ValueErrorType.IntegerExclusiveMaximum:
            return `Expected integer to be less than ${error.schema.exclusiveMaximum}`;
        case ValueErrorType.IntegerExclusiveMinimum:
            return `Expected integer to be greater than ${error.schema.exclusiveMinimum}`;
        case ValueErrorType.IntegerMaximum:
            return `Expected integer to be less or equal to ${error.schema.maximum}`;
        case ValueErrorType.IntegerMinimum:
            return `Expected integer to be greater or equal to ${error.schema.minimum}`;
        case ValueErrorType.IntegerMultipleOf:
            return `Expected integer to be a multiple of ${error.schema.multipleOf}`;
        case ValueErrorType.Integer:
            return 'Expected integer';
        case ValueErrorType.IntersectUnevaluatedProperties:
            return 'Unexpected property';
        case ValueErrorType.Intersect:
            return 'Expected all values to match';
        case ValueErrorType.Iterator:
            return 'Expected Iterator';
        case ValueErrorType.Literal:
            return `Expected ${typeof error.schema.const === 'string' ? `'${error.schema.const}'` : error.schema.const}`;
        case ValueErrorType.Never:
            return 'Never';
        case ValueErrorType.Not:
            return 'Value should not match';
        case ValueErrorType.Null:
            return 'Expected null';
        case ValueErrorType.NumberExclusiveMaximum:
            return `Expected number to be less than ${error.schema.exclusiveMaximum}`;
        case ValueErrorType.NumberExclusiveMinimum:
            return `Expected number to be greater than ${error.schema.exclusiveMinimum}`;
        case ValueErrorType.NumberMaximum:
            return `Expected number to be less or equal to ${error.schema.maximum}`;
        case ValueErrorType.NumberMinimum:
            return `Expected number to be greater or equal to ${error.schema.minimum}`;
        case ValueErrorType.NumberMultipleOf:
            return `Expected number to be a multiple of ${error.schema.multipleOf}`;
        case ValueErrorType.Number:
            return 'Expected number';
        case ValueErrorType.Object:
            return 'Expected object';
        case ValueErrorType.ObjectAdditionalProperties:
            return 'Unexpected property';
        case ValueErrorType.ObjectMaxProperties:
            return `Expected object to have no more than ${error.schema.maxProperties} properties`;
        case ValueErrorType.ObjectMinProperties:
            return `Expected object to have at least ${error.schema.minProperties} properties`;
        case ValueErrorType.ObjectRequiredProperty:
            return 'Expected required property';
        case ValueErrorType.Promise:
            return 'Expected Promise';
        case ValueErrorType.RegExp:
            return 'Expected string to match regular expression';
        case ValueErrorType.StringFormatUnknown:
            return `Unknown format '${error.schema.format}'`;
        case ValueErrorType.StringFormat:
            return `Expected string to match '${error.schema.format}' format`;
        case ValueErrorType.StringMaxLength:
            return `Expected string length less or equal to ${error.schema.maxLength}`;
        case ValueErrorType.StringMinLength:
            return `Expected string length greater or equal to ${error.schema.minLength}`;
        case ValueErrorType.StringPattern:
            return `Expected string to match '${error.schema.pattern}'`;
        case ValueErrorType.String:
            return 'Expected string';
        case ValueErrorType.Symbol:
            return 'Expected symbol';
        case ValueErrorType.TupleLength:
            return `Expected tuple to have ${error.schema.maxItems || 0} elements`;
        case ValueErrorType.Tuple:
            return 'Expected tuple';
        case ValueErrorType.Uint8ArrayMaxByteLength:
            return `Expected byte length less or equal to ${error.schema.maxByteLength}`;
        case ValueErrorType.Uint8ArrayMinByteLength:
            return `Expected byte length greater or equal to ${error.schema.minByteLength}`;
        case ValueErrorType.Uint8Array:
            return 'Expected Uint8Array';
        case ValueErrorType.Undefined:
            return 'Expected undefined';
        case ValueErrorType.Union:
            return 'Expected union value';
        case ValueErrorType.Void:
            return 'Expected void';
        case ValueErrorType.Kind:
            return `Expected kind '${error.schema[Kind]}'`;
        default:
            return 'Unknown error type';
    }
}
/** Manages error message providers */
let errorFunction = DefaultErrorFunction;
/** Sets the error function used to generate error messages. */
export function SetErrorFunction(callback) {
    errorFunction = callback;
}
/** Gets the error function used to generate error messages */
export function GetErrorFunction() {
    return errorFunction;
}
