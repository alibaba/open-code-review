import { JsonTypeBuilder } from './json.mjs';
import { Argument } from '../argument/index.mjs';
import { AsyncIterator } from '../async-iterator/index.mjs';
import { Awaited } from '../awaited/index.mjs';
import { BigInt } from '../bigint/index.mjs';
import { Constructor } from '../constructor/index.mjs';
import { ConstructorParameters } from '../constructor-parameters/index.mjs';
import { Date } from '../date/index.mjs';
import { Function as FunctionType } from '../function/index.mjs';
import { InstanceType } from '../instance-type/index.mjs';
import { Instantiate } from '../instantiate/index.mjs';
import { Iterator } from '../iterator/index.mjs';
import { Parameters } from '../parameters/index.mjs';
import { Promise } from '../promise/index.mjs';
import { RegExp } from '../regexp/index.mjs';
import { ReturnType } from '../return-type/index.mjs';
import { Symbol } from '../symbol/index.mjs';
import { Uint8Array } from '../uint8array/index.mjs';
import { Undefined } from '../undefined/index.mjs';
import { Void } from '../void/index.mjs';
/** JavaScript Type Builder with Static Resolution for TypeScript */
export class JavaScriptTypeBuilder extends JsonTypeBuilder {
    /** `[JavaScript]` Creates a Generic Argument Type */
    Argument(index) {
        return Argument(index);
    }
    /** `[JavaScript]` Creates a AsyncIterator type */
    AsyncIterator(items, options) {
        return AsyncIterator(items, options);
    }
    /** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
    Awaited(schema, options) {
        return Awaited(schema, options);
    }
    /** `[JavaScript]` Creates a BigInt type */
    BigInt(options) {
        return BigInt(options);
    }
    /** `[JavaScript]` Extracts the ConstructorParameters from the given Constructor type */
    ConstructorParameters(schema, options) {
        return ConstructorParameters(schema, options);
    }
    /** `[JavaScript]` Creates a Constructor type */
    Constructor(parameters, instanceType, options) {
        return Constructor(parameters, instanceType, options);
    }
    /** `[JavaScript]` Creates a Date type */
    Date(options = {}) {
        return Date(options);
    }
    /** `[JavaScript]` Creates a Function type */
    Function(parameters, returnType, options) {
        return FunctionType(parameters, returnType, options);
    }
    /** `[JavaScript]` Extracts the InstanceType from the given Constructor type */
    InstanceType(schema, options) {
        return InstanceType(schema, options);
    }
    /** `[JavaScript]` Instantiates a type with the given parameters */
    Instantiate(schema, parameters) {
        return Instantiate(schema, parameters);
    }
    /** `[JavaScript]` Creates an Iterator type */
    Iterator(items, options) {
        return Iterator(items, options);
    }
    /** `[JavaScript]` Extracts the Parameters from the given Function type */
    Parameters(schema, options) {
        return Parameters(schema, options);
    }
    /** `[JavaScript]` Creates a Promise type */
    Promise(item, options) {
        return Promise(item, options);
    }
    /** `[JavaScript]` Creates a RegExp type */
    RegExp(unresolved, options) {
        return RegExp(unresolved, options);
    }
    /** `[JavaScript]` Extracts the ReturnType from the given Function type */
    ReturnType(type, options) {
        return ReturnType(type, options);
    }
    /** `[JavaScript]` Creates a Symbol type */
    Symbol(options) {
        return Symbol(options);
    }
    /** `[JavaScript]` Creates a Undefined type */
    Undefined(options) {
        return Undefined(options);
    }
    /** `[JavaScript]` Creates a Uint8Array type */
    Uint8Array(options) {
        return Uint8Array(options);
    }
    /** `[JavaScript]` Creates a Void type */
    Void(options) {
        return Void(options);
    }
}
