import { Any } from '../any/index.mjs';
import { Array } from '../array/index.mjs';
import { Boolean } from '../boolean/index.mjs';
import { Composite } from '../composite/index.mjs';
import { Const } from '../const/index.mjs';
import { Enum } from '../enum/index.mjs';
import { Exclude } from '../exclude/index.mjs';
import { Extends } from '../extends/index.mjs';
import { Extract } from '../extract/index.mjs';
import { Index } from '../indexed/index.mjs';
import { Integer } from '../integer/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Capitalize, Uncapitalize, Lowercase, Uppercase } from '../intrinsic/index.mjs';
import { KeyOf } from '../keyof/index.mjs';
import { Literal } from '../literal/index.mjs';
import { Mapped } from '../mapped/index.mjs';
import { Never } from '../never/index.mjs';
import { Not } from '../not/index.mjs';
import { Null } from '../null/index.mjs';
import { Module } from '../module/index.mjs';
import { Number } from '../number/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Omit } from '../omit/index.mjs';
import { Optional } from '../optional/index.mjs';
import { Partial } from '../partial/index.mjs';
import { Pick } from '../pick/index.mjs';
import { Readonly } from '../readonly/index.mjs';
import { ReadonlyOptional } from '../readonly-optional/index.mjs';
import { Record } from '../record/index.mjs';
import { Recursive } from '../recursive/index.mjs';
import { Ref } from '../ref/index.mjs';
import { Required } from '../required/index.mjs';
import { Rest } from '../rest/index.mjs';
import { String } from '../string/index.mjs';
import { TemplateLiteral } from '../template-literal/index.mjs';
import { Transform } from '../transform/index.mjs';
import { Tuple } from '../tuple/index.mjs';
import { Union } from '../union/index.mjs';
import { Unknown } from '../unknown/index.mjs';
import { Unsafe } from '../unsafe/index.mjs';
/** Json Type Builder with Static Resolution for TypeScript */
export class JsonTypeBuilder {
    // ------------------------------------------------------------------------
    // Modifiers
    // ------------------------------------------------------------------------
    /** `[Json]` Creates a Readonly and Optional property */
    ReadonlyOptional(type) {
        return ReadonlyOptional(type);
    }
    /** `[Json]` Creates a Readonly property */
    Readonly(type, enable) {
        return Readonly(type, enable ?? true);
    }
    /** `[Json]` Creates a Optional property */
    Optional(type, enable) {
        return Optional(type, enable ?? true);
    }
    // ------------------------------------------------------------------------
    // Types
    // ------------------------------------------------------------------------
    /** `[Json]` Creates an Any type */
    Any(options) {
        return Any(options);
    }
    /** `[Json]` Creates an Array type */
    Array(items, options) {
        return Array(items, options);
    }
    /** `[Json]` Creates a Boolean type */
    Boolean(options) {
        return Boolean(options);
    }
    /** `[Json]` Intrinsic function to Capitalize LiteralString types */
    Capitalize(schema, options) {
        return Capitalize(schema, options);
    }
    /** `[Json]` Creates a Composite object type */
    Composite(schemas, options) {
        return Composite(schemas, options); // (error) TS 5.4.0-dev - review TComposite implementation
    }
    /** `[JavaScript]` Creates a readonly const type from the given value. */
    Const(value, options) {
        return Const(value, options);
    }
    /** `[Json]` Creates a Enum type */
    Enum(item, options) {
        return Enum(item, options);
    }
    /** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
    Exclude(unionType, excludedMembers, options) {
        return Exclude(unionType, excludedMembers, options);
    }
    /** `[Json]` Creates a Conditional type */
    Extends(L, R, T, F, options) {
        return Extends(L, R, T, F, options);
    }
    /** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
    Extract(type, union, options) {
        return Extract(type, union, options);
    }
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index(type, key, options) {
        return Index(type, key, options);
    }
    /** `[Json]` Creates an Integer type */
    Integer(options) {
        return Integer(options);
    }
    /** `[Json]` Creates an Intersect type */
    Intersect(types, options) {
        return Intersect(types, options);
    }
    /** `[Json]` Creates a KeyOf type */
    KeyOf(type, options) {
        return KeyOf(type, options);
    }
    /** `[Json]` Creates a Literal type */
    Literal(literalValue, options) {
        return Literal(literalValue, options);
    }
    /** `[Json]` Intrinsic function to Lowercase LiteralString types */
    Lowercase(type, options) {
        return Lowercase(type, options);
    }
    /** `[Json]` Creates a Mapped object type */
    Mapped(key, map, options) {
        return Mapped(key, map, options);
    }
    /** `[Json]` Creates a Type Definition Module. */
    Module(properties) {
        return Module(properties);
    }
    /** `[Json]` Creates a Never type */
    Never(options) {
        return Never(options);
    }
    /** `[Json]` Creates a Not type */
    Not(type, options) {
        return Not(type, options);
    }
    /** `[Json]` Creates a Null type */
    Null(options) {
        return Null(options);
    }
    /** `[Json]` Creates a Number type */
    Number(options) {
        return Number(options);
    }
    /** `[Json]` Creates an Object type */
    Object(properties, options) {
        return _Object_(properties, options);
    }
    /** `[Json]` Constructs a type whose keys are omitted from the given type */
    Omit(schema, selector, options) {
        return Omit(schema, selector, options);
    }
    /** `[Json]` Constructs a type where all properties are optional */
    Partial(type, options) {
        return Partial(type, options);
    }
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Pick(type, key, options) {
        return Pick(type, key, options);
    }
    /** `[Json]` Creates a Record type */
    Record(key, value, options) {
        return Record(key, value, options);
    }
    /** `[Json]` Creates a Recursive type */
    Recursive(callback, options) {
        return Recursive(callback, options);
    }
    /** `[Json]` Creates a Ref type. The referenced type must contain a $id */
    Ref(...args) {
        return Ref(args[0], args[1]);
    }
    /** `[Json]` Constructs a type where all properties are required */
    Required(type, options) {
        return Required(type, options);
    }
    /** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
    Rest(type) {
        return Rest(type);
    }
    /** `[Json]` Creates a String type */
    String(options) {
        return String(options);
    }
    /** `[Json]` Creates a TemplateLiteral type */
    TemplateLiteral(unresolved, options) {
        return TemplateLiteral(unresolved, options);
    }
    /** `[Json]` Creates a Transform type */
    Transform(type) {
        return Transform(type);
    }
    /** `[Json]` Creates a Tuple type */
    Tuple(types, options) {
        return Tuple(types, options);
    }
    /** `[Json]` Intrinsic function to Uncapitalize LiteralString types */
    Uncapitalize(type, options) {
        return Uncapitalize(type, options);
    }
    /** `[Json]` Creates a Union type */
    Union(types, options) {
        return Union(types, options);
    }
    /** `[Json]` Creates an Unknown type */
    Unknown(options) {
        return Unknown(options);
    }
    /** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
    Unsafe(options) {
        return Unsafe(options);
    }
    /** `[Json]` Intrinsic function to Uppercase LiteralString types */
    Uppercase(schema, options) {
        return Uppercase(schema, options);
    }
}
