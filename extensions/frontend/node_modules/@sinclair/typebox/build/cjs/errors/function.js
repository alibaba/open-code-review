"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.DefaultErrorFunction = DefaultErrorFunction;
exports.SetErrorFunction = SetErrorFunction;
exports.GetErrorFunction = GetErrorFunction;
const index_1 = require("../type/symbols/index");
const errors_1 = require("./errors");
/** Creates an error message using en-US as the default locale */
function DefaultErrorFunction(error) {
    switch (error.errorType) {
        case errors_1.ValueErrorType.ArrayContains:
            return 'Expected array to contain at least one matching value';
        case errors_1.ValueErrorType.ArrayMaxContains:
            return `Expected array to contain no more than ${error.schema.maxContains} matching values`;
        case errors_1.ValueErrorType.ArrayMinContains:
            return `Expected array to contain at least ${error.schema.minContains} matching values`;
        case errors_1.ValueErrorType.ArrayMaxItems:
            return `Expected array length to be less or equal to ${error.schema.maxItems}`;
        case errors_1.ValueErrorType.ArrayMinItems:
            return `Expected array length to be greater or equal to ${error.schema.minItems}`;
        case errors_1.ValueErrorType.ArrayUniqueItems:
            return 'Expected array elements to be unique';
        case errors_1.ValueErrorType.Array:
            return 'Expected array';
        case errors_1.ValueErrorType.AsyncIterator:
            return 'Expected AsyncIterator';
        case errors_1.ValueErrorType.BigIntExclusiveMaximum:
            return `Expected bigint to be less than ${error.schema.exclusiveMaximum}`;
        case errors_1.ValueErrorType.BigIntExclusiveMinimum:
            return `Expected bigint to be greater than ${error.schema.exclusiveMinimum}`;
        case errors_1.ValueErrorType.BigIntMaximum:
            return `Expected bigint to be less or equal to ${error.schema.maximum}`;
        case errors_1.ValueErrorType.BigIntMinimum:
            return `Expected bigint to be greater or equal to ${error.schema.minimum}`;
        case errors_1.ValueErrorType.BigIntMultipleOf:
            return `Expected bigint to be a multiple of ${error.schema.multipleOf}`;
        case errors_1.ValueErrorType.BigInt:
            return 'Expected bigint';
        case errors_1.ValueErrorType.Boolean:
            return 'Expected boolean';
        case errors_1.ValueErrorType.DateExclusiveMinimumTimestamp:
            return `Expected Date timestamp to be greater than ${error.schema.exclusiveMinimumTimestamp}`;
        case errors_1.ValueErrorType.DateExclusiveMaximumTimestamp:
            return `Expected Date timestamp to be less than ${error.schema.exclusiveMaximumTimestamp}`;
        case errors_1.ValueErrorType.DateMinimumTimestamp:
            return `Expected Date timestamp to be greater or equal to ${error.schema.minimumTimestamp}`;
        case errors_1.ValueErrorType.DateMaximumTimestamp:
            return `Expected Date timestamp to be less or equal to ${error.schema.maximumTimestamp}`;
        case errors_1.ValueErrorType.DateMultipleOfTimestamp:
            return `Expected Date timestamp to be a multiple of ${error.schema.multipleOfTimestamp}`;
        case errors_1.ValueErrorType.Date:
            return 'Expected Date';
        case errors_1.ValueErrorType.Function:
            return 'Expected function';
        case errors_1.ValueErrorType.IntegerExclusiveMaximum:
            return `Expected integer to be less than ${error.schema.exclusiveMaximum}`;
        case errors_1.ValueErrorType.IntegerExclusiveMinimum:
            return `Expected integer to be greater than ${error.schema.exclusiveMinimum}`;
        case errors_1.ValueErrorType.IntegerMaximum:
            return `Expected integer to be less or equal to ${error.schema.maximum}`;
        case errors_1.ValueErrorType.IntegerMinimum:
            return `Expected integer to be greater or equal to ${error.schema.minimum}`;
        case errors_1.ValueErrorType.IntegerMultipleOf:
            return `Expected integer to be a multiple of ${error.schema.multipleOf}`;
        case errors_1.ValueErrorType.Integer:
            return 'Expected integer';
        case errors_1.ValueErrorType.IntersectUnevaluatedProperties:
            return 'Unexpected property';
        case errors_1.ValueErrorType.Intersect:
            return 'Expected all values to match';
        case errors_1.ValueErrorType.Iterator:
            return 'Expected Iterator';
        case errors_1.ValueErrorType.Literal:
            return `Expected ${typeof error.schema.const === 'string' ? `'${error.schema.const}'` : error.schema.const}`;
        case errors_1.ValueErrorType.Never:
            return 'Never';
        case errors_1.ValueErrorType.Not:
            return 'Value should not match';
        case errors_1.ValueErrorType.Null:
            return 'Expected null';
        case errors_1.ValueErrorType.NumberExclusiveMaximum:
            return `Expected number to be less than ${error.schema.exclusiveMaximum}`;
        case errors_1.ValueErrorType.NumberExclusiveMinimum:
            return `Expected number to be greater than ${error.schema.exclusiveMinimum}`;
        case errors_1.ValueErrorType.NumberMaximum:
            return `Expected number to be less or equal to ${error.schema.maximum}`;
        case errors_1.ValueErrorType.NumberMinimum:
            return `Expected number to be greater or equal to ${error.schema.minimum}`;
        case errors_1.ValueErrorType.NumberMultipleOf:
            return `Expected number to be a multiple of ${error.schema.multipleOf}`;
        case errors_1.ValueErrorType.Number:
            return 'Expected number';
        case errors_1.ValueErrorType.Object:
            return 'Expected object';
        case errors_1.ValueErrorType.ObjectAdditionalProperties:
            return 'Unexpected property';
        case errors_1.ValueErrorType.ObjectMaxProperties:
            return `Expected object to have no more than ${error.schema.maxProperties} properties`;
        case errors_1.ValueErrorType.ObjectMinProperties:
            return `Expected object to have at least ${error.schema.minProperties} properties`;
        case errors_1.ValueErrorType.ObjectRequiredProperty:
            return 'Expected required property';
        case errors_1.ValueErrorType.Promise:
            return 'Expected Promise';
        case errors_1.ValueErrorType.RegExp:
            return 'Expected string to match regular expression';
        case errors_1.ValueErrorType.StringFormatUnknown:
            return `Unknown format '${error.schema.format}'`;
        case errors_1.ValueErrorType.StringFormat:
            return `Expected string to match '${error.schema.format}' format`;
        case errors_1.ValueErrorType.StringMaxLength:
            return `Expected string length less or equal to ${error.schema.maxLength}`;
        case errors_1.ValueErrorType.StringMinLength:
            return `Expected string length greater or equal to ${error.schema.minLength}`;
        case errors_1.ValueErrorType.StringPattern:
            return `Expected string to match '${error.schema.pattern}'`;
        case errors_1.ValueErrorType.String:
            return 'Expected string';
        case errors_1.ValueErrorType.Symbol:
            return 'Expected symbol';
        case errors_1.ValueErrorType.TupleLength:
            return `Expected tuple to have ${error.schema.maxItems || 0} elements`;
        case errors_1.ValueErrorType.Tuple:
            return 'Expected tuple';
        case errors_1.ValueErrorType.Uint8ArrayMaxByteLength:
            return `Expected byte length less or equal to ${error.schema.maxByteLength}`;
        case errors_1.ValueErrorType.Uint8ArrayMinByteLength:
            return `Expected byte length greater or equal to ${error.schema.minByteLength}`;
        case errors_1.ValueErrorType.Uint8Array:
            return 'Expected Uint8Array';
        case errors_1.ValueErrorType.Undefined:
            return 'Expected undefined';
        case errors_1.ValueErrorType.Union:
            return 'Expected union value';
        case errors_1.ValueErrorType.Void:
            return 'Expected void';
        case errors_1.ValueErrorType.Kind:
            return `Expected kind '${error.schema[index_1.Kind]}'`;
        default:
            return 'Unknown error type';
    }
}
/** Manages error message providers */
let errorFunction = DefaultErrorFunction;
/** Sets the error function used to generate error messages. */
function SetErrorFunction(callback) {
    errorFunction = callback;
}
/** Gets the error function used to generate error messages */
function GetErrorFunction() {
    return errorFunction;
}
