import fs, {} from 'node:fs';
import path from 'node:path';
import { EXTENSIONS, cjsRequire } from './constants.js';
export const tryPkg = (pkg) => {
    try {
        return cjsRequire.resolve(pkg);
    }
    catch { }
};
export const isPkgAvailable = (pkg) => Boolean(tryPkg(pkg));
const ANY_FILE_TYPES = new Set(['any', true]);
const isAnyFileType = (type) => ANY_FILE_TYPES.has(type);
export const tryFileStats = (filename, type = 'file', base = process.cwd()) => {
    if (!type) {
        type = 'file';
    }
    if (typeof filename === 'string') {
        const filepath = path.resolve(base, filename);
        let stats;
        try {
            stats = fs.statSync(filepath, { throwIfNoEntry: false });
        }
        catch { }
        return stats &&
            (isAnyFileType(type) ||
                (Array.isArray(type) ? type : [type]).some(type => stats[`is${type[0].toUpperCase()}${type.slice(1)}`]()))
            ? { filepath, stats }
            : undefined;
    }
    for (const file of filename ?? []) {
        const result = tryFileStats(file, type, base);
        if (result) {
            return result;
        }
    }
};
export const tryFile = (filename, type = 'file', base) => tryFileStats(filename, type, base)?.filepath ?? '';
export const tryExtensions = (filepath, extensions = EXTENSIONS) => {
    const ext = [...extensions, ''].find(ext => tryFile(filepath + ext));
    return ext == null ? '' : filepath + ext;
};
export const findUp = (entryOrOptions, options) => {
    if (typeof entryOrOptions === 'string') {
        options = {
            entry: entryOrOptions,
            ...options,
        };
    }
    else if (entryOrOptions) {
        options = options ? { ...entryOrOptions, ...options } : entryOrOptions;
    }
    let { entry = process.cwd(), search = 'package.json', type, stop, } = options ?? {};
    search = Array.isArray(search) ? search : [search];
    do {
        const searched = tryFile(search, type, entry);
        if (searched) {
            return searched;
        }
        const lastEntry = entry;
        entry = path.dirname(entry);
        if (entry === lastEntry) {
            break;
        }
    } while (!stop || entry !== stop);
    return '';
};
//# sourceMappingURL=helpers.js.map