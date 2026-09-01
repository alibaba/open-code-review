/// <reference types="node" preserve="true" />
export interface CjsRequire extends NodeJS.Require {
    <T>(id: string): T;
}
export declare const cjsRequire: CjsRequire;
export declare const EXTENSIONS: string[];
