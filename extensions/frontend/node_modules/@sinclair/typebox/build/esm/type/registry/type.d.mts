export type TypeRegistryValidationFunction<TSchema> = (schema: TSchema, value: unknown) => boolean;
/** Returns the entries in this registry */
export declare function Entries(): Map<string, TypeRegistryValidationFunction<any>>;
/** Clears all user defined types */
export declare function Clear(): void;
/** Deletes a registered type */
export declare function Delete(kind: string): boolean;
/** Returns true if this registry contains this kind */
export declare function Has(kind: string): boolean;
/** Sets a validation function for a user defined type */
export declare function Set<TSchema = unknown>(kind: string, func: TypeRegistryValidationFunction<TSchema>): void;
/** Gets a custom validation function for a user defined type */
export declare function Get(kind: string): TypeRegistryValidationFunction<any> | undefined;
