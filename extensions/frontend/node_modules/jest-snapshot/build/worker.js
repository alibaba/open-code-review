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

/***/ "./src/mockSerializer.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const serialize = (val, config, indentation, depth, refs, printer) => {
  // Serialize a non-default name, even if config.printFunctionName is false.
  const name = val.getMockName();
  const nameString = name === 'jest.fn()' ? '' : ` ${name}`;
  let callsString = '';
  if (val.mock.calls.length > 0) {
    const indentationNext = indentation + config.indent;
    callsString = ` {${config.spacingOuter}${indentationNext}"calls": ${printer(val.mock.calls, config, indentationNext, depth, refs)}${config.min ? ', ' : ','}${config.spacingOuter}${indentationNext}"results": ${printer(val.mock.results, config, indentationNext, depth, refs)}${config.min ? '' : ','}${config.spacingOuter}${indentation}}`;
  }
  return `[MockFunction${nameString}]${callsString}`;
};
exports.serialize = serialize;
const test = val => val && !!val._isMockFunction;
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getSerializers = exports.addSerializer = void 0;
var _prettyFormat = require("pretty-format");
var _mockSerializer = _interopRequireDefault(__webpack_require__("./src/mockSerializer.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  DOMCollection,
  DOMElement,
  Immutable,
  ReactElement,
  ReactTestComponent,
  AsymmetricMatcher
} = _prettyFormat.plugins;
let PLUGINS = [ReactTestComponent, ReactElement, DOMElement, DOMCollection, Immutable, _mockSerializer.default, AsymmetricMatcher];

// Prepend to list so the last added is the first tested.
const addSerializer = plugin => {
  PLUGINS = [plugin, ...PLUGINS];
};
exports.addSerializer = addSerializer;
const getSerializers = () => PLUGINS;
exports.getSerializers = getSerializers;

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.serialize = exports.removeLinesBeforeExternalMatcherTrap = exports.removeExtraLineBreaks = exports.processPrettierAst = exports.processInlineSnapshotsWithBabel = exports.minify = exports.groupSnapshotsByFile = exports.deserializeString = exports.deepMerge = exports.addExtraLineBreaks = void 0;
var fs = _interopRequireWildcard(require("graceful-fs"));
var _snapshotUtils = require("@jest/snapshot-utils");
var _prettyFormat = require("pretty-format");
var _plugins = __webpack_require__("./src/plugins.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestReadFile = globalThis[Symbol.for('jest-native-read-file')] || fs.readFileSync;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
function isObject(item) {
  return item != null && typeof item === 'object' && !Array.isArray(item);
}

// Add extra line breaks at beginning and end of multiline snapshot
// to make the content easier to read.
const addExtraLineBreaks = string => string.includes('\n') ? `\n${string}\n` : string;

// Remove extra line breaks at beginning and end of multiline snapshot.
// Instead of trim, which can remove additional newlines or spaces
// at beginning or end of the content from a custom serializer.
exports.addExtraLineBreaks = addExtraLineBreaks;
const removeExtraLineBreaks = string => string.length > 2 && string.startsWith('\n') && string.endsWith('\n') ? string.slice(1, -1) : string;
exports.removeExtraLineBreaks = removeExtraLineBreaks;
const removeLinesBeforeExternalMatcherTrap = stack => {
  const lines = stack.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    // It's a function name specified in `packages/expect/src/index.ts`
    // for external custom matchers.
    if (lines[i].includes('__EXTERNAL_MATCHER_TRAP__')) {
      return lines.slice(i + 1).join('\n');
    }
  }
  return stack;
};
exports.removeLinesBeforeExternalMatcherTrap = removeLinesBeforeExternalMatcherTrap;
const escapeRegex = true;
const printFunctionName = false;
const serialize = (val, indent = 2, formatOverrides = {}) => (0, _snapshotUtils.normalizeNewlines)((0, _prettyFormat.format)(val, {
  escapeRegex,
  indent,
  plugins: (0, _plugins.getSerializers)(),
  printFunctionName,
  ...formatOverrides
}));
exports.serialize = serialize;
const minify = val => (0, _prettyFormat.format)(val, {
  escapeRegex,
  min: true,
  plugins: (0, _plugins.getSerializers)(),
  printFunctionName
});

// Remove double quote marks and unescape double quotes and backslashes.
exports.minify = minify;
const deserializeString = stringified => stringified.slice(1, -1).replaceAll(/\\("|\\)/g, '$1');
exports.deserializeString = deserializeString;
const isAnyOrAnything = input => '$$typeof' in input && input.$$typeof === Symbol.for('jest.asymmetricMatcher') && ['Any', 'Anything'].includes(input.constructor.name);
const deepMergeArray = (target, source) => {
  const mergedOutput = [...target];
  for (const [index, sourceElement] of source.entries()) {
    const targetElement = mergedOutput[index];
    if (Array.isArray(target[index]) && Array.isArray(sourceElement)) {
      mergedOutput[index] = deepMergeArray(target[index], sourceElement);
    } else if (isObject(targetElement) && !isAnyOrAnything(sourceElement)) {
      mergedOutput[index] = deepMerge(target[index], sourceElement);
    } else {
      // Source does not exist in target or target is primitive and cannot be deep merged
      mergedOutput[index] = sourceElement;
    }
  }
  return mergedOutput;
};

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
const deepMerge = (target, source) => {
  if (isObject(target) && isObject(source)) {
    const mergedOutput = {
      ...target
    };
    for (const key of Object.keys(source)) {
      if (isObject(source[key]) && !source[key].$$typeof) {
        if (key in target) {
          mergedOutput[key] = deepMerge(target[key], source[key]);
        } else {
          Object.assign(mergedOutput, {
            [key]: source[key]
          });
        }
      } else if (Array.isArray(source[key])) {
        mergedOutput[key] = deepMergeArray(target[key], source[key]);
      } else {
        Object.assign(mergedOutput, {
          [key]: source[key]
        });
      }
    }
    return mergedOutput;
  } else if (Array.isArray(target) && Array.isArray(source)) {
    return deepMergeArray(target, source);
  }
  return target;
};
exports.deepMerge = deepMerge;
const indent = (snapshot, numIndents, indentation) => {
  const lines = snapshot.split('\n');
  // Prevent re-indentation of inline snapshots.
  if (lines.length >= 2 && lines[1].startsWith(indentation.repeat(numIndents + 1))) {
    return snapshot;
  }
  return lines.map((line, index) => {
    if (index === 0) {
      // First line is either a 1-line snapshot or a blank line.
      return line;
    } else if (index === lines.length - 1) {
      // The last line should be placed on the same level as the expect call.
      return indentation.repeat(numIndents) + line;
    } else {
      // Do not indent empty lines.
      if (line === '') {
        return line;
      }

      // Not last line, indent one level deeper than expect call.
      return indentation.repeat(numIndents + 1) + line;
    }
  }).join('\n');
};
const generate = require(require.resolve('@babel/generator', {
  [Symbol.for('jest-resolve-outside-vm-option')]: true
})).default;
const {
  parseSync,
  types
} = require(require.resolve('@babel/core', {
  [Symbol.for('jest-resolve-outside-vm-option')]: true
}));
const {
  isAwaitExpression,
  templateElement,
  templateLiteral,
  traverseFast,
  traverse
} = types;
const processInlineSnapshotsWithBabel = (snapshots, sourceFilePath, rootDir) => {
  const sourceFile = jestReadFile(sourceFilePath, 'utf8');

  // TypeScript projects may not have a babel config; make sure they can be parsed anyway.
  const presets = [require.resolve('babel-preset-current-node-syntax')];
  const plugins = [];
  if (/\.([cm]?ts|tsx)$/.test(sourceFilePath)) {
    plugins.push([require.resolve('@babel/plugin-syntax-typescript'), {
      isTSX: sourceFilePath.endsWith('x')
    },
    // unique name to make sure Babel does not complain about a possible duplicate plugin.
    'TypeScript syntax plugin added by Jest snapshot']);
  }

  // Record the matcher names seen during traversal and pass them down one
  // by one to formatting parser.
  const snapshotMatcherNames = [];
  let ast = null;
  try {
    ast = parseSync(sourceFile, {
      filename: sourceFilePath,
      plugins,
      presets,
      root: rootDir
    });
  } catch (error) {
    // attempt to recover from missing jsx plugin
    if (error.message.includes('@babel/plugin-syntax-jsx')) {
      try {
        const jsxSyntaxPlugin = [require.resolve('@babel/plugin-syntax-jsx'), {},
        // unique name to make sure Babel does not complain about a possible duplicate plugin.
        'JSX syntax plugin added by Jest snapshot'];
        ast = parseSync(sourceFile, {
          filename: sourceFilePath,
          plugins: [...plugins, jsxSyntaxPlugin],
          presets,
          root: rootDir
        });
      } catch {
        throw error;
      }
    } else {
      throw error;
    }
  }
  if (!ast) {
    throw new Error(`jest-snapshot: Failed to parse ${sourceFilePath}`);
  }
  traverseAst(snapshots, ast, snapshotMatcherNames);
  return {
    snapshotMatcherNames,
    sourceFile,
    // substitute in the snapshots in reverse order, so slice calculations aren't thrown off.
    sourceFileWithSnapshots: snapshots.reduceRight((sourceSoFar, nextSnapshot) => {
      const {
        node
      } = nextSnapshot;
      if (!node || typeof node.start !== 'number' || typeof node.end !== 'number') {
        throw new Error('Jest: no snapshot insert location found');
      }

      // A hack to prevent unexpected line breaks in the generated code
      node.loc.end.line = node.loc.start.line;
      return sourceSoFar.slice(0, node.start) + generate(node, {
        retainLines: true
      }).code.trim() + sourceSoFar.slice(node.end);
    }, sourceFile)
  };
};
exports.processInlineSnapshotsWithBabel = processInlineSnapshotsWithBabel;
const processPrettierAst = (ast, options, snapshotMatcherNames, keepNode) => {
  traverse(ast, (node, ancestors) => {
    if (node.type !== 'CallExpression') return;
    const {
      arguments: args,
      callee
    } = node;
    if (callee.type !== 'MemberExpression' || callee.property.type !== 'Identifier' || !snapshotMatcherNames.includes(callee.property.name) || !callee.loc || callee.computed) {
      return;
    }
    let snapshotIndex;
    let snapshot;
    for (const [i, node] of args.entries()) {
      if (node.type === 'TemplateLiteral') {
        snapshotIndex = i;
        snapshot = node.quasis[0].value.raw;
      }
    }
    if (snapshot === undefined) {
      return;
    }
    const parent = ancestors.at(-1).node;
    const startColumn = isAwaitExpression(parent) && parent.loc ? parent.loc.start.column : callee.loc.start.column;
    const useSpaces = !options?.useTabs;
    snapshot = indent(snapshot, Math.ceil(useSpaces ? startColumn / (options?.tabWidth ?? 1) :
    // Each tab is 2 characters.
    startColumn / 2), useSpaces ? ' '.repeat(options?.tabWidth ?? 1) : '\t');
    if (keepNode) {
      args[snapshotIndex].quasis[0].value.raw = snapshot;
    } else {
      const replacementNode = templateLiteral([templateElement({
        raw: snapshot
      })], []);
      args[snapshotIndex] = replacementNode;
    }
  });
};
exports.processPrettierAst = processPrettierAst;
const groupSnapshotsBy = createKey => snapshots => snapshots.reduce((object, inlineSnapshot) => {
  const key = createKey(inlineSnapshot);
  return {
    ...object,
    [key]: [...(object[key] || []), inlineSnapshot]
  };
}, {});
const groupSnapshotsByFrame = groupSnapshotsBy(({
  frame: {
    line,
    column
  }
}) => typeof line === 'number' && typeof column === 'number' ? `${line}:${column - 1}` : '');
const groupSnapshotsByFile = exports.groupSnapshotsByFile = groupSnapshotsBy(({
  frame: {
    file
  }
}) => file);
const traverseAst = (snapshots, ast, snapshotMatcherNames) => {
  const groupedSnapshots = groupSnapshotsByFrame(snapshots);
  const remainingSnapshots = new Set(snapshots.map(({
    snapshot
  }) => snapshot));
  traverseFast(ast, node => {
    if (node.type !== 'CallExpression') return;
    const {
      arguments: args,
      callee
    } = node;
    if (callee.type !== 'MemberExpression' || callee.property.type !== 'Identifier' || callee.property.loc == null) {
      return;
    }
    const {
      line,
      column
    } = callee.property.loc.start;
    const snapshotsForFrame = groupedSnapshots[`${line}:${column}`];
    if (!snapshotsForFrame) {
      return;
    }
    if (snapshotsForFrame.length > 1) {
      throw new Error('Jest: Multiple inline snapshots for the same call are not supported.');
    }
    const inlineSnapshot = snapshotsForFrame[0];
    inlineSnapshot.node = node;
    snapshotMatcherNames.push(callee.property.name);
    const snapshotIndex = args.findIndex(({
      type
    }) => type === 'TemplateLiteral' || type === 'StringLiteral');
    const {
      snapshot
    } = inlineSnapshot;
    remainingSnapshots.delete(snapshot);
    const replacementNode = templateLiteral([templateElement({
      raw: (0, _snapshotUtils.escapeBacktickString)(snapshot)
    })], []);
    if (snapshotIndex === -1) {
      args.push(replacementNode);
    } else {
      args[snapshotIndex] = replacementNode;
    }
  });
  if (remainingSnapshots.size > 0) {
    throw new Error("Jest: Couldn't locate all inline snapshots.");
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


var _synckit = require("synckit");
var _utils = __webpack_require__("./src/utils.ts");
var worker_Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
let prettier;
async function getInferredParser(filepath) {
  const fileInfo = await prettier.getFileInfo(filepath);
  return fileInfo.inferredParser;
}
(0, _synckit.runAsWorker)(async (prettierPath, filepath, sourceFileWithSnapshots, snapshotMatcherNames) => {
  prettier ??= require(/*webpackIgnore: true*/
  require.resolve(prettierPath, {
    [worker_Symbol.for('jest-resolve-outside-vm-option')]: true
  }));
  const config = await prettier.resolveConfig(filepath, {
    editorconfig: true
  });
  const inferredParser = typeof config?.parser === 'string' ? config.parser : await getInferredParser(filepath);
  if (!inferredParser) {
    throw new Error(`Could not infer Prettier parser for file ${filepath}`);
  }
  sourceFileWithSnapshots = await prettier.format(sourceFileWithSnapshots, {
    ...config,
    filepath,
    parser: inferredParser
  });
  const {
    ast,
    text: parsedSourceFileWithSnapshots
  } =
  // @ts-expect-error private API
  await prettier.__debug.parse(sourceFileWithSnapshots, {
    ...config,
    filepath,
    originalText: sourceFileWithSnapshots,
    parser: inferredParser
  });
  (0, _utils.processPrettierAst)(ast, config, snapshotMatcherNames, true);
  // Snapshots have now been inserted. Run prettier to make sure that the code is
  // formatted, except snapshot indentation. Snapshots cannot be formatted until
  // after the initial format because we don't know where the call expression
  // will be placed (specifically its indentation), so we have to do two
  // prettier.format calls back-to-back.
  // @ts-expect-error private API
  const formatAST = await prettier.__debug.formatAST(ast, {
    ...config,
    filepath,
    originalText: parsedSourceFileWithSnapshots,
    parser: inferredParser
  });
  return formatAST.formatted;
});
module.exports = __webpack_exports__;
/******/ })()
;