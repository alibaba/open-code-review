import { Kind } from '../symbols/index.mjs';
/** Fast undefined check used for properties of type undefined */
function Intersect(schema) {
    return schema.allOf.every((schema) => ExtendsUndefinedCheck(schema));
}
function Union(schema) {
    return schema.anyOf.some((schema) => ExtendsUndefinedCheck(schema));
}
function Not(schema) {
    return !ExtendsUndefinedCheck(schema.not);
}
/** Fast undefined check used for properties of type undefined */
// prettier-ignore
export function ExtendsUndefinedCheck(schema) {
    return (schema[Kind] === 'Intersect' ? Intersect(schema) :
        schema[Kind] === 'Union' ? Union(schema) :
            schema[Kind] === 'Not' ? Not(schema) :
                schema[Kind] === 'Undefined' ? true :
                    false);
}
