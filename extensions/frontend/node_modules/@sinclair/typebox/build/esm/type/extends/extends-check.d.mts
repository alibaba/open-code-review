import { type TSchema } from '../schema/index.mjs';
import { TypeBoxError } from '../error/index.mjs';
export declare class ExtendsResolverError extends TypeBoxError {
}
export declare enum ExtendsResult {
    Union = 0,
    True = 1,
    False = 2
}
export declare function ExtendsCheck(left: TSchema, right: TSchema): ExtendsResult;
