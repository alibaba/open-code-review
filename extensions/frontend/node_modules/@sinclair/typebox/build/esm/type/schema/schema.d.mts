import { Kind, Hint, ReadonlyKind, OptionalKind } from '../symbols/index.mjs';
export interface SchemaOptions {
    $schema?: string;
    /** Id for this schema */
    $id?: string;
    /** Title of this schema */
    title?: string;
    /** Description of this schema */
    description?: string;
    /** Default value for this schema */
    default?: any;
    /** Example values matching this schema */
    examples?: any;
    /** Optional annotation for readOnly */
    readOnly?: boolean;
    /** Optional annotation for writeOnly */
    writeOnly?: boolean;
    [prop: string]: any;
}
export interface TKind {
    [Kind]: string;
}
export interface TSchema extends TKind, SchemaOptions {
    [ReadonlyKind]?: string;
    [OptionalKind]?: string;
    [Hint]?: string;
    params: unknown[];
    static: unknown;
}
