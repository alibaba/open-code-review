import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsObject, IsSchema } from '../guard/kind.mjs';
// ------------------------------------------------------------------
// IntersectCreate
// ------------------------------------------------------------------
// prettier-ignore
export function IntersectCreate(T, options = {}) {
    const allObjects = T.every((schema) => IsObject(schema));
    const clonedUnevaluatedProperties = IsSchema(options.unevaluatedProperties)
        ? { unevaluatedProperties: options.unevaluatedProperties }
        : {};
    return CreateType((options.unevaluatedProperties === false || IsSchema(options.unevaluatedProperties) || allObjects
        ? { ...clonedUnevaluatedProperties, [Kind]: 'Intersect', type: 'object', allOf: T }
        : { ...clonedUnevaluatedProperties, [Kind]: 'Intersect', allOf: T }), options);
}
