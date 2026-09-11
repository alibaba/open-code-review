"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TypeSystemPolicy = void 0;
const index_1 = require("../value/guard/index");
var TypeSystemPolicy;
(function (TypeSystemPolicy) {
    // ------------------------------------------------------------------
    // TypeSystemPolicy: Instancing
    // ------------------------------------------------------------------
    /**
     * Configures the instantiation behavior of TypeBox types. The `default` option assigns raw JavaScript
     * references for embedded types, which may cause side effects if type properties are explicitly updated
     * outside the TypeBox type builder. The `clone` option creates copies of any shared types upon creation,
     * preventing unintended side effects. The `freeze` option applies `Object.freeze()` to the type, making
     * it fully readonly and immutable. Implementations should use `default` whenever possible, as it is the
     * fastest way to instantiate types. The default setting is `default`.
     */
    TypeSystemPolicy.InstanceMode = 'default';
    // ------------------------------------------------------------------
    // TypeSystemPolicy: Checking
    // ------------------------------------------------------------------
    /** Sets whether TypeBox should assert optional properties using the TypeScript `exactOptionalPropertyTypes` assertion policy. The default is `false` */
    TypeSystemPolicy.ExactOptionalPropertyTypes = false;
    /** Sets whether arrays should be treated as a kind of objects. The default is `false` */
    TypeSystemPolicy.AllowArrayObject = false;
    /** Sets whether `NaN` or `Infinity` should be treated as valid numeric values. The default is `false` */
    TypeSystemPolicy.AllowNaN = false;
    /** Sets whether `null` should validate for void types. The default is `false` */
    TypeSystemPolicy.AllowNullVoid = false;
    /** Checks this value using the ExactOptionalPropertyTypes policy */
    function IsExactOptionalProperty(value, key) {
        return TypeSystemPolicy.ExactOptionalPropertyTypes ? key in value : value[key] !== undefined;
    }
    TypeSystemPolicy.IsExactOptionalProperty = IsExactOptionalProperty;
    /** Checks this value using the AllowArrayObjects policy */
    function IsObjectLike(value) {
        const isObject = (0, index_1.IsObject)(value);
        return TypeSystemPolicy.AllowArrayObject ? isObject : isObject && !(0, index_1.IsArray)(value);
    }
    TypeSystemPolicy.IsObjectLike = IsObjectLike;
    /** Checks this value as a record using the AllowArrayObjects policy */
    function IsRecordLike(value) {
        return IsObjectLike(value) && !(value instanceof Date) && !(value instanceof Uint8Array);
    }
    TypeSystemPolicy.IsRecordLike = IsRecordLike;
    /** Checks this value using the AllowNaN policy */
    function IsNumberLike(value) {
        return TypeSystemPolicy.AllowNaN ? (0, index_1.IsNumber)(value) : Number.isFinite(value);
    }
    TypeSystemPolicy.IsNumberLike = IsNumberLike;
    /** Checks this value using the AllowVoidNull policy */
    function IsVoidLike(value) {
        const isUndefined = (0, index_1.IsUndefined)(value);
        return TypeSystemPolicy.AllowNullVoid ? isUndefined || value === null : isUndefined;
    }
    TypeSystemPolicy.IsVoidLike = IsVoidLike;
})(TypeSystemPolicy || (exports.TypeSystemPolicy = TypeSystemPolicy = {}));
