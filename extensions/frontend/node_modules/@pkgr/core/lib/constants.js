import { createRequire } from 'node:module';
export const cjsRequire = typeof require === 'function' ? require : createRequire(import.meta.url);
const DEFAULT_EXTENSIONS = cjsRequire.extensions
    ?
        Object.keys(cjsRequire.extensions)
    :
        ['.js', '.json', '.node'];
export const EXTENSIONS = ['.ts', '.tsx', ...DEFAULT_EXTENSIONS];
//# sourceMappingURL=constants.js.map