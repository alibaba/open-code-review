"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.JsonTypeBuilder = void 0;
const index_1 = require("../any/index");
const index_2 = require("../array/index");
const index_3 = require("../boolean/index");
const index_4 = require("../composite/index");
const index_5 = require("../const/index");
const index_6 = require("../enum/index");
const index_7 = require("../exclude/index");
const index_8 = require("../extends/index");
const index_9 = require("../extract/index");
const index_10 = require("../indexed/index");
const index_11 = require("../integer/index");
const index_12 = require("../intersect/index");
const index_13 = require("../intrinsic/index");
const index_14 = require("../keyof/index");
const index_15 = require("../literal/index");
const index_16 = require("../mapped/index");
const index_17 = require("../never/index");
const index_18 = require("../not/index");
const index_19 = require("../null/index");
const index_20 = require("../module/index");
const index_21 = require("../number/index");
const index_22 = require("../object/index");
const index_23 = require("../omit/index");
const index_24 = require("../optional/index");
const index_25 = require("../partial/index");
const index_26 = require("../pick/index");
const index_27 = require("../readonly/index");
const index_28 = require("../readonly-optional/index");
const index_29 = require("../record/index");
const index_30 = require("../recursive/index");
const index_31 = require("../ref/index");
const index_32 = require("../required/index");
const index_33 = require("../rest/index");
const index_34 = require("../string/index");
const index_35 = require("../template-literal/index");
const index_36 = require("../transform/index");
const index_37 = require("../tuple/index");
const index_38 = require("../union/index");
const index_39 = require("../unknown/index");
const index_40 = require("../unsafe/index");
/** Json Type Builder with Static Resolution for TypeScript */
class JsonTypeBuilder {
    // ------------------------------------------------------------------------
    // Modifiers
    // ------------------------------------------------------------------------
    /** `[Json]` Creates a Readonly and Optional property */
    ReadonlyOptional(type) {
        return (0, index_28.ReadonlyOptional)(type);
    }
    /** `[Json]` Creates a Readonly property */
    Readonly(type, enable) {
        return (0, index_27.Readonly)(type, enable ?? true);
    }
    /** `[Json]` Creates a Optional property */
    Optional(type, enable) {
        return (0, index_24.Optional)(type, enable ?? true);
    }
    // ------------------------------------------------------------------------
    // Types
    // ------------------------------------------------------------------------
    /** `[Json]` Creates an Any type */
    Any(options) {
        return (0, index_1.Any)(options);
    }
    /** `[Json]` Creates an Array type */
    Array(items, options) {
        return (0, index_2.Array)(items, options);
    }
    /** `[Json]` Creates a Boolean type */
    Boolean(options) {
        return (0, index_3.Boolean)(options);
    }
    /** `[Json]` Intrinsic function to Capitalize LiteralString types */
    Capitalize(schema, options) {
        return (0, index_13.Capitalize)(schema, options);
    }
    /** `[Json]` Creates a Composite object type */
    Composite(schemas, options) {
        return (0, index_4.Composite)(schemas, options); // (error) TS 5.4.0-dev - review TComposite implementation
    }
    /** `[JavaScript]` Creates a readonly const type from the given value. */
    Const(value, options) {
        return (0, index_5.Const)(value, options);
    }
    /** `[Json]` Creates a Enum type */
    Enum(item, options) {
        return (0, index_6.Enum)(item, options);
    }
    /** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
    Exclude(unionType, excludedMembers, options) {
        return (0, index_7.Exclude)(unionType, excludedMembers, options);
    }
    /** `[Json]` Creates a Conditional type */
    Extends(L, R, T, F, options) {
        return (0, index_8.Extends)(L, R, T, F, options);
    }
    /** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
    Extract(type, union, options) {
        return (0, index_9.Extract)(type, union, options);
    }
    /** `[Json]` Returns an Indexed property type for the given keys */
    Index(type, key, options) {
        return (0, index_10.Index)(type, key, options);
    }
    /** `[Json]` Creates an Integer type */
    Integer(options) {
        return (0, index_11.Integer)(options);
    }
    /** `[Json]` Creates an Intersect type */
    Intersect(types, options) {
        return (0, index_12.Intersect)(types, options);
    }
    /** `[Json]` Creates a KeyOf type */
    KeyOf(type, options) {
        return (0, index_14.KeyOf)(type, options);
    }
    /** `[Json]` Creates a Literal type */
    Literal(literalValue, options) {
        return (0, index_15.Literal)(literalValue, options);
    }
    /** `[Json]` Intrinsic function to Lowercase LiteralString types */
    Lowercase(type, options) {
        return (0, index_13.Lowercase)(type, options);
    }
    /** `[Json]` Creates a Mapped object type */
    Mapped(key, map, options) {
        return (0, index_16.Mapped)(key, map, options);
    }
    /** `[Json]` Creates a Type Definition Module. */
    Module(properties) {
        return (0, index_20.Module)(properties);
    }
    /** `[Json]` Creates a Never type */
    Never(options) {
        return (0, index_17.Never)(options);
    }
    /** `[Json]` Creates a Not type */
    Not(type, options) {
        return (0, index_18.Not)(type, options);
    }
    /** `[Json]` Creates a Null type */
    Null(options) {
        return (0, index_19.Null)(options);
    }
    /** `[Json]` Creates a Number type */
    Number(options) {
        return (0, index_21.Number)(options);
    }
    /** `[Json]` Creates an Object type */
    Object(properties, options) {
        return (0, index_22.Object)(properties, options);
    }
    /** `[Json]` Constructs a type whose keys are omitted from the given type */
    Omit(schema, selector, options) {
        return (0, index_23.Omit)(schema, selector, options);
    }
    /** `[Json]` Constructs a type where all properties are optional */
    Partial(type, options) {
        return (0, index_25.Partial)(type, options);
    }
    /** `[Json]` Constructs a type whose keys are picked from the given type */
    Pick(type, key, options) {
        return (0, index_26.Pick)(type, key, options);
    }
    /** `[Json]` Creates a Record type */
    Record(key, value, options) {
        return (0, index_29.Record)(key, value, options);
    }
    /** `[Json]` Creates a Recursive type */
    Recursive(callback, options) {
        return (0, index_30.Recursive)(callback, options);
    }
    /** `[Json]` Creates a Ref type. The referenced type must contain a $id */
    Ref(...args) {
        return (0, index_31.Ref)(args[0], args[1]);
    }
    /** `[Json]` Constructs a type where all properties are required */
    Required(type, options) {
        return (0, index_32.Required)(type, options);
    }
    /** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
    Rest(type) {
        return (0, index_33.Rest)(type);
    }
    /** `[Json]` Creates a String type */
    String(options) {
        return (0, index_34.String)(options);
    }
    /** `[Json]` Creates a TemplateLiteral type */
    TemplateLiteral(unresolved, options) {
        return (0, index_35.TemplateLiteral)(unresolved, options);
    }
    /** `[Json]` Creates a Transform type */
    Transform(type) {
        return (0, index_36.Transform)(type);
    }
    /** `[Json]` Creates a Tuple type */
    Tuple(types, options) {
        return (0, index_37.Tuple)(types, options);
    }
    /** `[Json]` Intrinsic function to Uncapitalize LiteralString types */
    Uncapitalize(type, options) {
        return (0, index_13.Uncapitalize)(type, options);
    }
    /** `[Json]` Creates a Union type */
    Union(types, options) {
        return (0, index_38.Union)(types, options);
    }
    /** `[Json]` Creates an Unknown type */
    Unknown(options) {
        return (0, index_39.Unknown)(options);
    }
    /** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
    Unsafe(options) {
        return (0, index_40.Unsafe)(options);
    }
    /** `[Json]` Intrinsic function to Uppercase LiteralString types */
    Uppercase(schema, options) {
        return (0, index_13.Uppercase)(schema, options);
    }
}
exports.JsonTypeBuilder = JsonTypeBuilder;
