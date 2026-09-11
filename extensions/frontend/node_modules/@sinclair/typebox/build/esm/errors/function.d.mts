import { TSchema } from '../type/schema/index.mjs';
import { ValueErrorIterator, ValueErrorType } from './errors.mjs';
/** Creates an error message using en-US as the default locale */
export declare function DefaultErrorFunction(error: ErrorFunctionParameter): string;
export type ErrorFunctionParameter = {
    /** The type of validation error */
    errorType: ValueErrorType;
    /** The path of the error */
    path: string;
    /** The schema associated with the error */
    schema: TSchema;
    /** The value associated with the error */
    value: unknown;
    /** Interior errors for this error */
    errors: ValueErrorIterator[];
};
export type ErrorFunction = (parameter: ErrorFunctionParameter) => string;
/** Sets the error function used to generate error messages. */
export declare function SetErrorFunction(callback: ErrorFunction): void;
/** Gets the error function used to generate error messages */
export declare function GetErrorFunction(): ErrorFunction;
