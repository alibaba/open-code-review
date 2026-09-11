"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtendsUndefinedCheck = ExtendsUndefinedCheck;
const index_1 = require("../symbols/index");
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
function ExtendsUndefinedCheck(schema) {
    return (schema[index_1.Kind] === 'Intersect' ? Intersect(schema) :
        schema[index_1.Kind] === 'Union' ? Union(schema) :
            schema[index_1.Kind] === 'Not' ? Not(schema) :
                schema[index_1.Kind] === 'Undefined' ? true :
                    false);
}
