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

/***/ "./src/blacklist.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// This list is compiled after the MDN list of the most common MIME types (see
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/
// Complete_list_of_MIME_types).
//
// Only MIME types starting with "image/", "video/", "audio/" and "font/" are
// reflected in the list. Adding "application/" is too risky since some text
// file formats (like ".js" and ".json") have an "application/" MIME type.
//
// Feel free to add any extensions that cannot be a Haste module.

const extensions = new Set([
// JSONs are never haste modules, except for "package.json", which is handled.
'.json',
// Image extensions.
'.bmp', '.gif', '.ico', '.jpeg', '.jpg', '.png', '.svg', '.tiff', '.tif', '.webp',
// Video extensions.
'.avi', '.mp4', '.mpeg', '.mpg', '.ogv', '.webm', '.3gp', '.3g2',
// Audio extensions.
'.aac', '.midi', '.mid', '.mp3', '.oga', '.wav', '.3gp', '.3g2',
// Font extensions.
'.eot', '.otf', '.ttf', '.woff', '.woff2']);
var _default = exports["default"] = extensions;

/***/ },

/***/ "./src/constants.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/*
 * This file exports a set of constants that are used for Jest's haste map
 * serialization. On very large repositories, the haste map cache becomes very
 * large to the point where it is the largest overhead in starting up Jest.
 *
 * This constant key map allows to keep the map smaller without having to build
 * a custom serialization library.
 */

/* eslint-disable sort-keys */
const constants = {
  /* dependency serialization */
  DEPENDENCY_DELIM: '\0',
  /* file map attributes */
  ID: 0,
  MTIME: 1,
  SIZE: 2,
  VISITED: 3,
  DEPENDENCIES: 4,
  SHA1: 5,
  /* module map attributes */
  PATH: 0,
  TYPE: 1,
  /* module types */
  MODULE: 0,
  PACKAGE: 1,
  /* platforms */
  GENERIC_PLATFORM: 'g',
  NATIVE_PLATFORM: 'native'
};
/* eslint-enable */
var _default = exports["default"] = constants;

/***/ },

/***/ "./src/lib/dependencyExtractor.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.extractor = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NOT_A_DOT = '(?<!\\.\\s*)';
const CAPTURE_STRING_LITERAL = pos => `([\`'"])([^'"\`]*?)(?:\\${pos})`;
const WORD_SEPARATOR = '\\b';
const LEFT_PARENTHESIS = '\\(';
const RIGHT_PARENTHESIS = '\\)';
const WHITESPACE = '\\s*';
const OPTIONAL_COMMA = '(:?,\\s*)?';
function createRegExp(parts, flags) {
  return new RegExp(parts.join(''), flags);
}
function alternatives(...parts) {
  return `(?:${parts.join('|')})`;
}
function functionCallStart(...names) {
  return [NOT_A_DOT, WORD_SEPARATOR, alternatives(...names), WHITESPACE, LEFT_PARENTHESIS, WHITESPACE];
}
const BLOCK_COMMENT_RE = /\/\*[^]*?\*\//g;
const LINE_COMMENT_RE = /\/\/.*/g;
const REQUIRE_OR_DYNAMIC_IMPORT_RE = createRegExp([...functionCallStart('require', 'import'), CAPTURE_STRING_LITERAL(1), WHITESPACE, OPTIONAL_COMMA, RIGHT_PARENTHESIS], 'g');
const IMPORT_OR_EXPORT_RE = createRegExp(['\\b(?:import|export)\\s+(?!type(?:of)?\\s+)(?:[^\'"]+\\s+from\\s+)?', CAPTURE_STRING_LITERAL(1)], 'g');
const JEST_EXTENSIONS_RE = createRegExp([...functionCallStart('jest\\s*\\.\\s*(?:requireActual|requireMock|createMockFromModule)'), CAPTURE_STRING_LITERAL(1), WHITESPACE, OPTIONAL_COMMA, RIGHT_PARENTHESIS], 'g');
const extractor = exports.extractor = {
  extract(code) {
    const dependencies = new Set();
    const addDependency = (match, _, dep) => {
      dependencies.add(dep);
      return match;
    };
    code.replaceAll(BLOCK_COMMENT_RE, '').replaceAll(LINE_COMMENT_RE, '').replace(IMPORT_OR_EXPORT_RE, addDependency).replace(REQUIRE_OR_DYNAMIC_IMPORT_RE, addDependency).replace(JEST_EXTENSIONS_RE, addDependency);
    return dependencies;
  }
};

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
exports.getSha1 = getSha1;
exports.worker = worker;
function _nodeCrypto() {
  const data = require("node:crypto");
  _nodeCrypto = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
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
var _blacklist = _interopRequireDefault(__webpack_require__("./src/blacklist.ts"));
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var _dependencyExtractor = __webpack_require__("./src/lib/dependencyExtractor.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const PACKAGE_JSON = `${path().sep}package.json`;
function sha1hex(content) {
  return (0, _nodeCrypto().createHash)('sha1').update(content).digest('hex');
}
async function worker(data) {
  const hasteImpl = data.hasteImplModulePath ? require(data.hasteImplModulePath) : null;
  let content;
  let dependencies;
  let id;
  let module;
  let sha1;
  const {
    computeDependencies,
    computeSha1,
    rootDir,
    filePath
  } = data;
  const getContent = () => {
    if (content === undefined) {
      content = fs().readFileSync(filePath, 'utf8');
    }
    return content;
  };
  if (filePath.endsWith(PACKAGE_JSON)) {
    // Process a package.json that is returned as a PACKAGE type with its name.
    try {
      const fileData = JSON.parse(getContent());
      if (fileData.name) {
        const relativeFilePath = path().relative(rootDir, filePath);
        id = fileData.name;
        module = [relativeFilePath, _constants.default.PACKAGE];
      }
    } catch (error) {
      throw new Error(`Cannot parse ${filePath} as JSON: ${error.message}`);
    }
  } else if (!_blacklist.default.has(filePath.slice(filePath.lastIndexOf('.')))) {
    // Process a random file that is returned as a MODULE.
    if (hasteImpl) {
      id = hasteImpl.getHasteName(filePath);
    }
    if (computeDependencies) {
      const content = getContent();
      const extractor = data.dependencyExtractor ? await (0, _jestUtil().requireOrImportModule)(data.dependencyExtractor, false) : _dependencyExtractor.extractor;
      dependencies = [...extractor.extract(content, filePath, _dependencyExtractor.extractor.extract)];
    }
    if (id) {
      const relativeFilePath = path().relative(rootDir, filePath);
      module = [relativeFilePath, _constants.default.MODULE];
    }
  }

  // If a SHA-1 is requested on update, compute it.
  if (computeSha1) {
    sha1 = sha1hex(content || fs().readFileSync(filePath));
  }
  return {
    dependencies,
    id,
    module,
    sha1
  };
}
async function getSha1(data) {
  const sha1 = data.computeSha1 ? sha1hex(fs().readFileSync(data.filePath)) : null;
  return {
    dependencies: undefined,
    id: undefined,
    module: undefined,
    sha1
  };
}
})();

module.exports = __webpack_exports__;
/******/ })()
;