"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Object = void 0;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
/** Creates a RequiredArray derived from the given TProperties value. */
function RequiredArray(properties) {
    return globalThis.Object.keys(properties).filter((key) => !(0, kind_1.IsOptional)(properties[key]));
}
/** `[Json]` Creates an Object type */
function _Object_(properties, options) {
    const required = RequiredArray(properties);
    const schema = required.length > 0 ? { [index_1.Kind]: 'Object', type: 'object', required, properties } : { [index_1.Kind]: 'Object', type: 'object', properties };
    return (0, type_1.CreateType)(schema, options);
}
// ------------------------------------------------------------------
// TypeScript 7: CommonJS TS2441
//
// TypeScript generates a CommonJS shim that patches local variables
// via an unqualified reference to Object (e.g. the __esModule shim's
// Object.defineProperty call). Other compiler tools have been
// observed patching in the same way, but TypeScript 7 has correctly
// begun flagging variables named Object in CommonJS, since they
// conflict with the shim and can cause "used before definition"
// errors. This is CommonJS-specific; no such shim exists for ESM.
//
// TypeBox works around this using a `var` declaration, which is
// known to avoid use-before-definition errors. TypeBox 1.x employs
// a similar strategy.
// ------------------------------------------------------------------
/** `[Json]` Creates an Object type */
// @ts-ignore - error TS2441: Duplicate identifier 'Object'. Compiler reserves name 'Object' in top level scope of a module.
exports.Object = _Object_;
