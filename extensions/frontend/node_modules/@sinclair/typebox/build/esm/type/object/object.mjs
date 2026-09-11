import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsOptional } from '../guard/kind.mjs';
/** Creates a RequiredArray derived from the given TProperties value. */
function RequiredArray(properties) {
    return globalThis.Object.keys(properties).filter((key) => !IsOptional(properties[key]));
}
/** `[Json]` Creates an Object type */
function _Object_(properties, options) {
    const required = RequiredArray(properties);
    const schema = required.length > 0 ? { [Kind]: 'Object', type: 'object', required, properties } : { [Kind]: 'Object', type: 'object', properties };
    return CreateType(schema, options);
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
export var Object = _Object_;
