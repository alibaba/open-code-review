import * as t from '../type/index.mjs';
import { Type } from './parser.mjs';
/** `[Experimental]` Parses type expressions into TypeBox types but does not infer */
// prettier-ignore
export function NoInfer(...args) {
    const withContext = typeof args[0] === 'string' ? false : true;
    const [context, code, options] = withContext ? [args[0], args[1], args[2] || {}] : [{}, args[0], args[1] || {}];
    const result = Type(code, context)[0];
    return t.KindGuard.IsSchema(result)
        ? t.CloneType(result, options)
        : t.Never(options);
}
/** `[Experimental]` Parses type expressions into TypeBox types */
export function Syntax(...args) {
    return NoInfer.apply(null, args);
}
