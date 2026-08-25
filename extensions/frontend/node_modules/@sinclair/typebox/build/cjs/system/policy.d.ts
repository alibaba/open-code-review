export declare namespace TypeSystemPolicy {
    /**
     * Configures the instantiation behavior of TypeBox types. The `default` option assigns raw JavaScript
     * references for embedded types, which may cause side effects if type properties are explicitly updated
     * outside the TypeBox type builder. The `clone` option creates copies of any shared types upon creation,
     * preventing unintended side effects. The `freeze` option applies `Object.freeze()` to the type, making
     * it fully readonly and immutable. Implementations should use `default` whenever possible, as it is the
     * fastest way to instantiate types. The default setting is `default`.
     */
    let InstanceMode: 'default' | 'clone' | 'freeze';
    /** Sets whether TypeBox should assert optional properties using the TypeScript `exactOptionalPropertyTypes` assertion policy. The default is `false` */
    let ExactOptionalPropertyTypes: boolean;
    /** Sets whether arrays should be treated as a kind of objects. The default is `false` */
    let AllowArrayObject: boolean;
    /** Sets whether `NaN` or `Infinity` should be treated as valid numeric values. The default is `false` */
    let AllowNaN: boolean;
    /** Sets whether `null` should validate for void types. The default is `false` */
    let AllowNullVoid: boolean;
    /** Checks this value using the ExactOptionalPropertyTypes policy */
    function IsExactOptionalProperty(value: Record<keyof any, unknown>, key: string): boolean;
    /** Checks this value using the AllowArrayObjects policy */
    function IsObjectLike(value: unknown): value is Record<keyof any, unknown>;
    /** Checks this value as a record using the AllowArrayObjects policy */
    function IsRecordLike(value: unknown): value is Record<keyof any, unknown>;
    /** Checks this value using the AllowNaN policy */
    function IsNumberLike(value: unknown): value is number;
    /** Checks this value using the AllowVoidNull policy */
    function IsVoidLike(value: unknown): value is void;
}
