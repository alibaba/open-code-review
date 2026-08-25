import { type Stats } from 'node:fs';
export declare const tryPkg: (pkg: string) => string | undefined;
export declare const isPkgAvailable: (pkg: string) => boolean;
export type FileTypeBase = 'blockDevice' | 'characterDevice' | 'directory' | 'FIFO' | 'file' | 'socket' | 'symbolicLink';
export type FileType = Capitalize<FileTypeBase> | FileTypeBase;
export type FileTypes = FileType | FileType[] | boolean | 'any';
export declare const tryFileStats: (filename?: string[] | string, type?: FileTypes, base?: string) => {
    filepath: string;
    stats: Stats;
} | undefined;
export declare const tryFile: (filename?: string[] | string, type?: FileTypes, base?: string) => string;
export declare const tryExtensions: (filepath: string, extensions?: string[]) => string;
export interface FindUpOptions {
    entry?: string;
    search?: string[] | string;
    type?: FileTypes;
    stop?: string;
}
export declare const findUp: (entryOrOptions?: FindUpOptions | string, options?: FindUpOptions) => string;
