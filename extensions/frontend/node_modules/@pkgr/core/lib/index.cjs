'use strict';

var node_module = require('node:module');
var fs = require('node:fs');
var path = require('node:path');

var _documentCurrentScript = typeof document !== 'undefined' ? document.currentScript : null;
const cjsRequire = typeof require === "function" ? require : node_module.createRequire((typeof document === 'undefined' ? require('u' + 'rl').pathToFileURL(__filename).href : (_documentCurrentScript && _documentCurrentScript.tagName.toUpperCase() === 'SCRIPT' && _documentCurrentScript.src || new URL('index.cjs', document.baseURI).href)));
const DEFAULT_EXTENSIONS = cjsRequire.extensions ? (
  // eslint-disable-next-line sonarjs/deprecation
  Object.keys(cjsRequire.extensions)
) : (
  // `require.extensions` could be `undefined` - #430
  [".js", ".json", ".node"]
);
const EXTENSIONS = [".ts", ".tsx", ...DEFAULT_EXTENSIONS];

const tryPkg = (pkg) => {
  try {
    return cjsRequire.resolve(pkg);
  } catch {
  }
};
const isPkgAvailable = (pkg) => Boolean(tryPkg(pkg));
const ANY_FILE_TYPES = /* @__PURE__ */ new Set(["any", true]);
const isAnyFileType = (type) => ANY_FILE_TYPES.has(type);
const tryFileStats = (filename, type = "file", base = process.cwd()) => {
  if (!type) {
    type = "file";
  }
  if (typeof filename === "string") {
    const filepath = path.resolve(base, filename);
    let stats;
    try {
      stats = fs.statSync(filepath, { throwIfNoEntry: false });
    } catch {
    }
    return stats && (isAnyFileType(type) || (Array.isArray(type) ? type : [type]).some(
      (type2) => stats[`is${type2[0].toUpperCase()}${type2.slice(1)}`]()
    )) ? { filepath, stats } : void 0;
  }
  for (const file of filename ?? []) {
    const result = tryFileStats(file, type, base);
    if (result) {
      return result;
    }
  }
};
const tryFile = (filename, type = "file", base) => tryFileStats(filename, type, base)?.filepath ?? "";
const tryExtensions = (filepath, extensions = EXTENSIONS) => {
  const ext = [...extensions, ""].find((ext2) => tryFile(filepath + ext2));
  return ext == null ? "" : filepath + ext;
};
const findUp = (entryOrOptions, options) => {
  if (typeof entryOrOptions === "string") {
    options = {
      entry: entryOrOptions,
      ...options
    };
  } else if (entryOrOptions) {
    options = options ? { ...entryOrOptions, ...options } : entryOrOptions;
  }
  let {
    entry = process.cwd(),
    search = "package.json",
    type,
    stop
  } = options ?? {};
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
  return "";
};

exports.EXTENSIONS = EXTENSIONS;
exports.cjsRequire = cjsRequire;
exports.findUp = findUp;
exports.isPkgAvailable = isPkgAvailable;
exports.tryExtensions = tryExtensions;
exports.tryFile = tryFile;
exports.tryFileStats = tryFileStats;
exports.tryPkg = tryPkg;
