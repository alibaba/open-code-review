export type FormatRegistryValidationFunction = (value: string) => boolean;
/** Returns the entries in this registry */
export declare function Entries(): Map<string, FormatRegistryValidationFunction>;
/** Clears all user defined string formats */
export declare function Clear(): void;
/** Deletes a registered format */
export declare function Delete(format: string): boolean;
/** Returns true if the user defined string format exists */
export declare function Has(format: string): boolean;
/** Sets a validation function for a user defined string format */
export declare function Set(format: string, func: FormatRegistryValidationFunction): void;
/** Gets a validation function for a user defined string format */
export declare function Get(format: string): FormatRegistryValidationFunction | undefined;
