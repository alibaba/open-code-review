/*!
 * /**
 *  * Copyright (c) Meta Platforms, Inc. and affiliates.
 *  *
 *  * This source code is licensed under the MIT license found in the
 *  * LICENSE file in the root directory of this source tree.
 *  * /
 */
/******/ (() => { // webpackBootstrap
/******/ 	"use strict";
/******/ 	var __webpack_modules__ = ({

/***/ "./src/git.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _execa() {
  const data = _interopRequireDefault(require("execa"));
  _execa = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const findChangedFilesUsingCommand = async (args, cwd) => {
  const result = await (0, _execa().default)('git', args, {
    cwd
  });
  return result.stdout.split('\n').filter(s => s !== '').map(changedPath => path().resolve(cwd, changedPath));
};
const adapter = {
  findChangedFiles: async (cwd, options) => {
    const changedSince = options.withAncestor === true ? 'HEAD^' : options.changedSince;
    const includePaths = (options.includePaths ?? []).map(absoluteRoot => path().normalize(path().relative(cwd, absoluteRoot)));
    if (options.lastCommit === true) {
      return findChangedFilesUsingCommand(['show', '--name-only', '--pretty=format:', 'HEAD', '--', ...includePaths], cwd);
    }
    if (changedSince != null && changedSince.length > 0) {
      const [committed, staged, unstaged] = await Promise.all([findChangedFilesUsingCommand(['diff', '--name-only', `${changedSince}...HEAD`, '--', ...includePaths], cwd), findChangedFilesUsingCommand(['diff', '--cached', '--name-only', '--', ...includePaths], cwd), findChangedFilesUsingCommand(['ls-files', '--other', '--modified', '--exclude-standard', '--', ...includePaths], cwd)]);
      return [...committed, ...staged, ...unstaged];
    }
    const [staged, unstaged] = await Promise.all([findChangedFilesUsingCommand(['diff', '--cached', '--name-only', '--', ...includePaths], cwd), findChangedFilesUsingCommand(['ls-files', '--other', '--modified', '--exclude-standard', '--', ...includePaths], cwd)]);
    return [...staged, ...unstaged];
  },
  getRoot: async cwd => {
    const options = ['rev-parse', '--show-cdup'];
    try {
      const result = await (0, _execa().default)('git', options, {
        cwd
      });
      return path().resolve(cwd, result.stdout);
    } catch {
      return null;
    }
  }
};
var _default = exports["default"] = adapter;

/***/ },

/***/ "./src/hg.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _execa() {
  const data = _interopRequireDefault(require("execa"));
  _execa = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const env = {
  ...process.env,
  HGPLAIN: '1'
};
const adapter = {
  findChangedFiles: async (cwd, options) => {
    const includePaths = options.includePaths ?? [];
    const args = ['status', '-amnu'];
    if (options.withAncestor === true) {
      args.push('--rev', 'first(min(!public() & ::.)^+.^)');
    } else if (options.changedSince != null && options.changedSince.length > 0) {
      args.push('--rev', `ancestor(., ${options.changedSince})`);
    } else if (options.lastCommit === true) {
      args.push('--change', '.');
    }
    args.push(...includePaths);
    const result = await (0, _execa().default)('hg', args, {
      cwd,
      env
    });
    return result.stdout.split('\n').filter(s => s !== '').map(changedPath => path().resolve(cwd, changedPath));
  },
  getRoot: async cwd => {
    try {
      const result = await (0, _execa().default)('hg', ['root'], {
        cwd,
        env
      });
      return result.stdout;
    } catch {
      return null;
    }
  }
};
var _default = exports["default"] = adapter;

/***/ },

/***/ "./src/sl.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _execa() {
  const data = _interopRequireDefault(require("execa"));
  _execa = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

/**
 * Disable any configuration settings that might change Sapling's default output.
 * More info in `sl help environment`.  _HG_PLAIN is intentional
 */
const env = {
  ...process.env,
  HGPLAIN: '1'
};

// Whether `sl` is a steam locomotive or not
let isSteamLocomotive = false;
const adapter = {
  findChangedFiles: async (cwd, options) => {
    const includePaths = options.includePaths ?? [];
    const args = ['status', '-amnu'];
    if (options.withAncestor === true) {
      args.push('--rev', 'first(min(!public() & ::.)^+.^)');
    } else if (options.changedSince != null && options.changedSince.length > 0) {
      args.push('--rev', `ancestor(., ${options.changedSince})`);
    } else if (options.lastCommit === true) {
      args.push('--change', '.');
    }
    args.push(...includePaths);
    const result = await (0, _execa().default)('sl', args, {
      cwd,
      env
    });
    return result.stdout.split('\n').filter(s => s !== '').map(changedPath => path().resolve(cwd, changedPath));
  },
  getRoot: async cwd => {
    if (isSteamLocomotive) {
      return null;
    }
    try {
      const subprocess = (0, _execa().default)('sl', ['root'], {
        cwd,
        env
      });

      // Check if we're calling sl (steam locomotive) instead of sl (sapling)
      // by looking for the escape character in the first chunk of data.
      if (subprocess.stdout) {
        subprocess.stdout.once('data', data => {
          data = Buffer.isBuffer(data) ? data.toString() : data;
          if (data.codePointAt(0) === 27) {
            subprocess.cancel();
            isSteamLocomotive = true;
          }
        });
      }
      const result = await subprocess;
      if (result.killed && isSteamLocomotive) {
        return null;
      }
      return result.stdout;
    } catch {
      return null;
    }
  }
};
var _default = exports["default"] = adapter;

/***/ }

/******/ 	});
/************************************************************************/
/******/ 	// The module cache
/******/ 	var __webpack_module_cache__ = {};
/******/ 	
/******/ 	// The require function
/******/ 	function __webpack_require__(moduleId) {
/******/ 		// Check if module is in cache
/******/ 		var cachedModule = __webpack_module_cache__[moduleId];
/******/ 		if (cachedModule !== undefined) {
/******/ 			return cachedModule.exports;
/******/ 		}
/******/ 		// Create a new module (and put it into the cache)
/******/ 		var module = __webpack_module_cache__[moduleId] = {
/******/ 			// no module.id needed
/******/ 			// no module.loaded needed
/******/ 			exports: {}
/******/ 		};
/******/ 	
/******/ 		// Execute the module function
/******/ 		__webpack_modules__[moduleId](module, module.exports, __webpack_require__);
/******/ 	
/******/ 		// Return the exports of the module
/******/ 		return module.exports;
/******/ 	}
/******/ 	
/************************************************************************/
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getChangedFilesForRoots = exports.findRepos = void 0;
function _pLimit() {
  const data = _interopRequireDefault(require("p-limit"));
  _pLimit = function () {
    return data;
  };
  return data;
}
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _git = _interopRequireDefault(__webpack_require__("./src/git.ts"));
var _hg = _interopRequireDefault(__webpack_require__("./src/hg.ts"));
var _sl = _interopRequireDefault(__webpack_require__("./src/sl.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// This is an arbitrary number. The main goal is to prevent projects with
// many roots (50+) from spawning too many processes at once.
const mutex = (0, _pLimit().default)(5);
const findGitRoot = dir => mutex(() => _git.default.getRoot(dir));
const findHgRoot = dir => mutex(() => _hg.default.getRoot(dir));
const findSlRoot = dir => mutex(() => _sl.default.getRoot(dir));
const getChangedFilesForRoots = async (roots, options) => {
  const repos = await findRepos(roots);
  const changedFilesOptions = {
    includePaths: roots,
    ...options
  };
  const gitPromises = Array.from(repos.git, repo => _git.default.findChangedFiles(repo, changedFilesOptions));
  const hgPromises = Array.from(repos.hg, repo => _hg.default.findChangedFiles(repo, changedFilesOptions));
  const slPromises = Array.from(repos.sl, repo => _sl.default.findChangedFiles(repo, changedFilesOptions));
  const allVcs = await Promise.all([...gitPromises, ...hgPromises, ...slPromises]);
  const changedFiles = allVcs.reduce((allFiles, changedFilesInTheRepo) => {
    for (const file of changedFilesInTheRepo) {
      allFiles.add(file);
    }
    return allFiles;
  }, new Set());
  return {
    changedFiles,
    repos
  };
};
exports.getChangedFilesForRoots = getChangedFilesForRoots;
const findRepos = async roots => {
  const [gitRepos, hgRepos, slRepos] = await Promise.all([Promise.all(roots.map(findGitRoot)), Promise.all(roots.map(findHgRoot)), Promise.all(roots.map(findSlRoot))]);
  return {
    git: new Set(gitRepos.filter(_jestUtil().isNonNullable)),
    hg: new Set(hgRepos.filter(_jestUtil().isNonNullable)),
    sl: new Set(slRepos.filter(_jestUtil().isNonNullable))
  };
};
exports.findRepos = findRepos;
})();

module.exports = __webpack_exports__;
/******/ })()
;