"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.JavaScriptTypeBuilder = void 0;
const json_1 = require("./json");
const index_1 = require("../argument/index");
const index_2 = require("../async-iterator/index");
const index_3 = require("../awaited/index");
const index_4 = require("../bigint/index");
const index_5 = require("../constructor/index");
const index_6 = require("../constructor-parameters/index");
const index_7 = require("../date/index");
const index_8 = require("../function/index");
const index_9 = require("../instance-type/index");
const index_10 = require("../instantiate/index");
const index_11 = require("../iterator/index");
const index_12 = require("../parameters/index");
const index_13 = require("../promise/index");
const index_14 = require("../regexp/index");
const index_15 = require("../return-type/index");
const index_16 = require("../symbol/index");
const index_17 = require("../uint8array/index");
const index_18 = require("../undefined/index");
const index_19 = require("../void/index");
/** JavaScript Type Builder with Static Resolution for TypeScript */
class JavaScriptTypeBuilder extends json_1.JsonTypeBuilder {
    /** `[JavaScript]` Creates a Generic Argument Type */
    Argument(index) {
        return (0, index_1.Argument)(index);
    }
    /** `[JavaScript]` Creates a AsyncIterator type */
    AsyncIterator(items, options) {
        return (0, index_2.AsyncIterator)(items, options);
    }
    /** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
    Awaited(schema, options) {
        return (0, index_3.Awaited)(schema, options);
    }
    /** `[JavaScript]` Creates a BigInt type */
    BigInt(options) {
        return (0, index_4.BigInt)(options);
    }
    /** `[JavaScript]` Extracts the ConstructorParameters from the given Constructor type */
    ConstructorParameters(schema, options) {
        return (0, index_6.ConstructorParameters)(schema, options);
    }
    /** `[JavaScript]` Creates a Constructor type */
    Constructor(parameters, instanceType, options) {
        return (0, index_5.Constructor)(parameters, instanceType, options);
    }
    /** `[JavaScript]` Creates a Date type */
    Date(options = {}) {
        return (0, index_7.Date)(options);
    }
    /** `[JavaScript]` Creates a Function type */
    Function(parameters, returnType, options) {
        return (0, index_8.Function)(parameters, returnType, options);
    }
    /** `[JavaScript]` Extracts the InstanceType from the given Constructor type */
    InstanceType(schema, options) {
        return (0, index_9.InstanceType)(schema, options);
    }
    /** `[JavaScript]` Instantiates a type with the given parameters */
    Instantiate(schema, parameters) {
        return (0, index_10.Instantiate)(schema, parameters);
    }
    /** `[JavaScript]` Creates an Iterator type */
    Iterator(items, options) {
        return (0, index_11.Iterator)(items, options);
    }
    /** `[JavaScript]` Extracts the Parameters from the given Function type */
    Parameters(schema, options) {
        return (0, index_12.Parameters)(schema, options);
    }
    /** `[JavaScript]` Creates a Promise type */
    Promise(item, options) {
        return (0, index_13.Promise)(item, options);
    }
    /** `[JavaScript]` Creates a RegExp type */
    RegExp(unresolved, options) {
        return (0, index_14.RegExp)(unresolved, options);
    }
    /** `[JavaScript]` Extracts the ReturnType from the given Function type */
    ReturnType(type, options) {
        return (0, index_15.ReturnType)(type, options);
    }
    /** `[JavaScript]` Creates a Symbol type */
    Symbol(options) {
        return (0, index_16.Symbol)(options);
    }
    /** `[JavaScript]` Creates a Undefined type */
    Undefined(options) {
        return (0, index_18.Undefined)(options);
    }
    /** `[JavaScript]` Creates a Uint8Array type */
    Uint8Array(options) {
        return (0, index_17.Uint8Array)(options);
    }
    /** `[JavaScript]` Creates a Void type */
    Void(options) {
        return (0, index_19.Void)(options);
    }
}
exports.JavaScriptTypeBuilder = JavaScriptTypeBuilder;
