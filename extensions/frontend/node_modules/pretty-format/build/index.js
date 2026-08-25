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

/***/ "./src/collections.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printIteratorEntries = printIteratorEntries;
exports.printIteratorValues = printIteratorValues;
exports.printListItems = printListItems;
exports.printObjectProperties = printObjectProperties;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const getKeysOfEnumerableProperties = (object, compareKeys) => {
  const rawKeys = Object.keys(object);
  const keys = compareKeys === null ? rawKeys : rawKeys.sort(compareKeys);
  if (Object.getOwnPropertySymbols) {
    for (const symbol of Object.getOwnPropertySymbols(object)) {
      if (Object.getOwnPropertyDescriptor(object, symbol).enumerable) {
        keys.push(symbol);
      }
    }
  }
  return keys;
};

/**
 * Return entries (for example, of a map)
 * with spacing, indentation, and comma
 * without surrounding punctuation (for example, braces)
 */
function printIteratorEntries(iterator, config, indentation, depth, refs, printer,
// Too bad, so sad that separator for ECMAScript Map has been ' => '
// What a distracting diff if you change a data structure to/from
// ECMAScript Object or Immutable.Map/OrderedMap which use the default.
separator = ': ') {
  let result = '';
  let width = 0;
  let current = iterator.next();
  if (!current.done) {
    result += config.spacingOuter;
    const indentationNext = indentation + config.indent;
    while (!current.done) {
      result += indentationNext;
      if (width++ === config.maxWidth) {
        result += '…';
        break;
      }
      const name = printer(current.value[0], config, indentationNext, depth, refs);
      const value = printer(current.value[1], config, indentationNext, depth, refs);
      result += name + separator + value;
      current = iterator.next();
      if (!current.done) {
        result += `,${config.spacingInner}`;
      } else if (!config.min) {
        result += ',';
      }
    }
    result += config.spacingOuter + indentation;
  }
  return result;
}

/**
 * Return values (for example, of a set)
 * with spacing, indentation, and comma
 * without surrounding punctuation (braces or brackets)
 */
function printIteratorValues(iterator, config, indentation, depth, refs, printer) {
  let result = '';
  let width = 0;
  let current = iterator.next();
  if (!current.done) {
    result += config.spacingOuter;
    const indentationNext = indentation + config.indent;
    while (!current.done) {
      result += indentationNext;
      if (width++ === config.maxWidth) {
        result += '…';
        break;
      }
      result += printer(current.value, config, indentationNext, depth, refs);
      current = iterator.next();
      if (!current.done) {
        result += `,${config.spacingInner}`;
      } else if (!config.min) {
        result += ',';
      }
    }
    result += config.spacingOuter + indentation;
  }
  return result;
}

/**
 * Return items (for example, of an array)
 * with spacing, indentation, and comma
 * without surrounding punctuation (for example, brackets)
 **/
function printListItems(list, config, indentation, depth, refs, printer) {
  let result = '';
  list = list instanceof ArrayBuffer ? new DataView(list) : list;
  const isDataView = l => l instanceof DataView;
  const length = isDataView(list) ? list.byteLength : list.length;
  if (length > 0) {
    result += config.spacingOuter;
    const indentationNext = indentation + config.indent;
    for (let i = 0; i < length; i++) {
      result += indentationNext;
      if (i === config.maxWidth) {
        result += '…';
        break;
      }
      if (isDataView(list) || i in list) {
        result += printer(isDataView(list) ? list.getInt8(i) : list[i], config, indentationNext, depth, refs);
      }
      if (i < length - 1) {
        result += `,${config.spacingInner}`;
      } else if (!config.min) {
        result += ',';
      }
    }
    result += config.spacingOuter + indentation;
  }
  return result;
}

/**
 * Return properties of an object
 * with spacing, indentation, and comma
 * without surrounding punctuation (for example, braces)
 */
function printObjectProperties(val, config, indentation, depth, refs, printer) {
  let result = '';
  const keys = getKeysOfEnumerableProperties(val, config.compareKeys);
  if (keys.length > 0) {
    result += config.spacingOuter;
    const indentationNext = indentation + config.indent;
    for (let i = 0; i < keys.length; i++) {
      const key = keys[i];
      const name = printer(key, config, indentationNext, depth, refs);
      const value = printer(val[key], config, indentationNext, depth, refs);
      result += `${indentationNext + name}: ${value}`;
      if (i < keys.length - 1) {
        result += `,${config.spacingInner}`;
      } else if (!config.min) {
        result += ',';
      }
    }
    result += config.spacingOuter + indentation;
  }
  return result;
}

/***/ },

/***/ "./src/plugins/AsymmetricMatcher.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var _collections = __webpack_require__("./src/collections.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const asymmetricMatcher = typeof Symbol === 'function' && Symbol.for ? Symbol.for('jest.asymmetricMatcher') : 0x13_57_a5;
const SPACE = ' ';
const serialize = (val, config, indentation, depth, refs, printer) => {
  const stringedValue = val.toString();
  if (stringedValue === 'ArrayContaining' || stringedValue === 'ArrayNotContaining') {
    if (++depth > config.maxDepth) {
      return `[${stringedValue}]`;
    }
    return `${stringedValue + SPACE}[${(0, _collections.printListItems)(val.sample, config, indentation, depth, refs, printer)}]`;
  }
  if (stringedValue === 'ObjectContaining' || stringedValue === 'ObjectNotContaining') {
    if (++depth > config.maxDepth) {
      return `[${stringedValue}]`;
    }
    return `${stringedValue + SPACE}{${(0, _collections.printObjectProperties)(val.sample, config, indentation, depth, refs, printer)}}`;
  }
  if (stringedValue === 'StringMatching' || stringedValue === 'StringNotMatching') {
    return stringedValue + SPACE + printer(val.sample, config, indentation, depth, refs);
  }
  if (stringedValue === 'StringContaining' || stringedValue === 'StringNotContaining') {
    return stringedValue + SPACE + printer(val.sample, config, indentation, depth, refs);
  }
  if (stringedValue === 'ArrayOf' || stringedValue === 'NotArrayOf') {
    if (++depth > config.maxDepth) {
      return `[${stringedValue}]`;
    }
    return `${stringedValue + SPACE}${printer(val.sample, config, indentation, depth, refs)}`;
  }
  if (typeof val.toAsymmetricMatcher !== 'function') {
    throw new TypeError(`Asymmetric matcher ${val.constructor.name} does not implement toAsymmetricMatcher()`);
  }
  return val.toAsymmetricMatcher();
};
exports.serialize = serialize;
const test = val => val && val.$$typeof === asymmetricMatcher;
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/DOMCollection.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var _collections = __webpack_require__("./src/collections.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const SPACE = ' ';
const OBJECT_NAMES = new Set(['DOMStringMap', 'NamedNodeMap']);
const ARRAY_REGEXP = /^(HTML\w*Collection|NodeList)$/;
const testName = name => OBJECT_NAMES.has(name) || ARRAY_REGEXP.test(name);
const test = val => val && val.constructor && !!val.constructor.name && testName(val.constructor.name);
exports.test = test;
const isNamedNodeMap = collection => collection.constructor.name === 'NamedNodeMap';
const serialize = (collection, config, indentation, depth, refs, printer) => {
  const name = collection.constructor.name;
  if (++depth > config.maxDepth) {
    return `[${name}]`;
  }
  return (config.min ? '' : name + SPACE) + (OBJECT_NAMES.has(name) ? `{${(0, _collections.printObjectProperties)(isNamedNodeMap(collection) ? [...collection].reduce((props, attribute) => {
    props[attribute.name] = attribute.value;
    return props;
  }, {}) : {
    ...collection
  }, config, indentation, depth, refs, printer)}}` : `[${(0, _collections.printListItems)([...collection], config, indentation, depth, refs, printer)}]`);
};
exports.serialize = serialize;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/DOMElement.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var _markup = __webpack_require__("./src/plugins/lib/markup.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const ELEMENT_NODE = 1;
const TEXT_NODE = 3;
const COMMENT_NODE = 8;
const FRAGMENT_NODE = 11;
const ELEMENT_REGEXP = /^((HTML|SVG)\w*)?Element$/;
const testHasAttribute = val => {
  try {
    return typeof val.hasAttribute === 'function' && val.hasAttribute('is');
  } catch {
    return false;
  }
};
const isCustomElement = val => {
  const tagName = val?.tagName;
  return typeof tagName === 'string' && tagName.includes('-') || testHasAttribute(val);
};
const testNode = val => {
  const constructorName = val.constructor.name;
  const {
    nodeType
  } = val;
  return nodeType === ELEMENT_NODE && (ELEMENT_REGEXP.test(constructorName) || isCustomElement(val)) || nodeType === TEXT_NODE && constructorName === 'Text' || nodeType === COMMENT_NODE && constructorName === 'Comment' || nodeType === FRAGMENT_NODE && constructorName === 'DocumentFragment';
};
const test = val => (val?.constructor?.name || isCustomElement(val)) && testNode(val);
exports.test = test;
function nodeIsText(node) {
  return node.nodeType === TEXT_NODE;
}
function nodeIsComment(node) {
  return node.nodeType === COMMENT_NODE;
}
function nodeIsFragment(node) {
  return node.nodeType === FRAGMENT_NODE;
}
const serialize = (node, config, indentation, depth, refs, printer) => {
  if (nodeIsText(node)) {
    return (0, _markup.printText)(node.data, config);
  }
  if (nodeIsComment(node)) {
    return (0, _markup.printComment)(node.data, config);
  }
  const type = nodeIsFragment(node) ? 'DocumentFragment' : node.tagName.toLowerCase();
  if (++depth > config.maxDepth) {
    return (0, _markup.printElementAsLeaf)(type, config);
  }
  return (0, _markup.printElement)(type, (0, _markup.printProps)(nodeIsFragment(node) ? [] : Array.from(node.attributes, attr => attr.name).sort(), nodeIsFragment(node) ? {} : [...node.attributes].reduce((props, attribute) => {
    props[attribute.name] = attribute.value;
    return props;
  }, {}), config, indentation + config.indent, depth, refs, printer), (0, _markup.printChildren)(Array.prototype.slice.call(node.childNodes || node.children), config, indentation + config.indent, depth, refs, printer), config, indentation);
};
exports.serialize = serialize;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/Immutable.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var _collections = __webpack_require__("./src/collections.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// SENTINEL constants are from https://github.com/facebook/immutable-js
const IS_ITERABLE_SENTINEL = '@@__IMMUTABLE_ITERABLE__@@';
const IS_LIST_SENTINEL = '@@__IMMUTABLE_LIST__@@';
const IS_KEYED_SENTINEL = '@@__IMMUTABLE_KEYED__@@';
const IS_MAP_SENTINEL = '@@__IMMUTABLE_MAP__@@';
const IS_ORDERED_SENTINEL = '@@__IMMUTABLE_ORDERED__@@';
const IS_RECORD_SENTINEL = '@@__IMMUTABLE_RECORD__@@'; // immutable v4
const IS_SEQ_SENTINEL = '@@__IMMUTABLE_SEQ__@@';
const IS_SET_SENTINEL = '@@__IMMUTABLE_SET__@@';
const IS_STACK_SENTINEL = '@@__IMMUTABLE_STACK__@@';
const getImmutableName = name => `Immutable.${name}`;
const printAsLeaf = name => `[${name}]`;
const SPACE = ' ';
const LAZY = '…'; // Seq is lazy if it calls a method like filter

const printImmutableEntries = (val, config, indentation, depth, refs, printer, type) => ++depth > config.maxDepth ? printAsLeaf(getImmutableName(type)) : `${getImmutableName(type) + SPACE}{${(0, _collections.printIteratorEntries)(val.entries(), config, indentation, depth, refs, printer)}}`;

// Record has an entries method because it is a collection in immutable v3.
// Return an iterator for Immutable Record from version v3 or v4.
function getRecordEntries(val) {
  let i = 0;
  return {
    next() {
      if (i < val._keys.length) {
        const key = val._keys[i++];
        return {
          done: false,
          value: [key, val.get(key)]
        };
      }
      return {
        done: true,
        value: undefined
      };
    }
  };
}
const printImmutableRecord = (val, config, indentation, depth, refs, printer) => {
  // _name property is defined only for an Immutable Record instance
  // which was constructed with a second optional descriptive name arg
  const name = getImmutableName(val._name || 'Record');
  return ++depth > config.maxDepth ? printAsLeaf(name) : `${name + SPACE}{${(0, _collections.printIteratorEntries)(getRecordEntries(val), config, indentation, depth, refs, printer)}}`;
};
const printImmutableSeq = (val, config, indentation, depth, refs, printer) => {
  const name = getImmutableName('Seq');
  if (++depth > config.maxDepth) {
    return printAsLeaf(name);
  }
  if (val[IS_KEYED_SENTINEL]) {
    return `${name + SPACE}{${
    // from Immutable collection of entries or from ECMAScript object
    val._iter || val._object ? (0, _collections.printIteratorEntries)(val.entries(), config, indentation, depth, refs, printer) : LAZY}}`;
  }
  return `${name + SPACE}[${val._iter ||
  // from Immutable collection of values
  val._array ||
  // from ECMAScript array
  val._collection ||
  // from ECMAScript collection in immutable v4
  val._iterable // from ECMAScript collection in immutable v3
  ? (0, _collections.printIteratorValues)(val.values(), config, indentation, depth, refs, printer) : LAZY}]`;
};
const printImmutableValues = (val, config, indentation, depth, refs, printer, type) => ++depth > config.maxDepth ? printAsLeaf(getImmutableName(type)) : `${getImmutableName(type) + SPACE}[${(0, _collections.printIteratorValues)(val.values(), config, indentation, depth, refs, printer)}]`;
const serialize = (val, config, indentation, depth, refs, printer) => {
  if (val[IS_MAP_SENTINEL]) {
    return printImmutableEntries(val, config, indentation, depth, refs, printer, val[IS_ORDERED_SENTINEL] ? 'OrderedMap' : 'Map');
  }
  if (val[IS_LIST_SENTINEL]) {
    return printImmutableValues(val, config, indentation, depth, refs, printer, 'List');
  }
  if (val[IS_SET_SENTINEL]) {
    return printImmutableValues(val, config, indentation, depth, refs, printer, val[IS_ORDERED_SENTINEL] ? 'OrderedSet' : 'Set');
  }
  if (val[IS_STACK_SENTINEL]) {
    return printImmutableValues(val, config, indentation, depth, refs, printer, 'Stack');
  }
  if (val[IS_SEQ_SENTINEL]) {
    return printImmutableSeq(val, config, indentation, depth, refs, printer);
  }

  // For compatibility with immutable v3 and v4, let record be the default.
  return printImmutableRecord(val, config, indentation, depth, refs, printer);
};

// Explicitly comparing sentinel properties to true avoids false positive
// when mock identity-obj-proxy returns the key as the value for any key.
exports.serialize = serialize;
const test = val => val && (val[IS_ITERABLE_SENTINEL] === true || val[IS_RECORD_SENTINEL] === true);
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/ReactElement.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var ReactIs18 = _interopRequireWildcard(require("react-is-18"));
var ReactIs19 = _interopRequireWildcard(require("react-is-19"));
var _markup = __webpack_require__("./src/plugins/lib/markup.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isElement = val => ReactIs18.isElement(val) || ReactIs19.isElement(val);
const isFragment = val => ReactIs18.isFragment(val) || ReactIs19.isFragment(val);
const isSuspense = val => ReactIs18.isSuspense(val) || ReactIs19.isSuspense(val);
const isContextProvider = val => ReactIs18.isContextProvider(val) || ReactIs19.isContextProvider(val);
const isContextConsumer = val => ReactIs18.isContextConsumer(val) || ReactIs19.isContextConsumer(val);
const isForwardRef = val => ReactIs18.isForwardRef(val) || ReactIs19.isForwardRef(val);
const isMemo = val => ReactIs18.isMemo(val) || ReactIs19.isMemo(val);

// Given element.props.children, or subtree during recursive traversal,
// return flattened array of children.
const getChildren = (arg, children = []) => {
  if (Array.isArray(arg)) {
    for (const item of arg) {
      getChildren(item, children);
    }
  } else if (arg != null && arg !== false && arg !== '') {
    children.push(arg);
  }
  return children;
};
const getType = element => {
  const type = element.type;
  if (typeof type === 'string') {
    return type;
  }
  if (typeof type === 'function') {
    return type.displayName || type.name || 'Unknown';
  }
  if (isFragment(element)) {
    return 'React.Fragment';
  }
  if (isSuspense(element)) {
    return 'React.Suspense';
  }
  if (typeof type === 'object' && type !== null) {
    if (isContextProvider(element)) {
      return 'Context.Provider';
    }
    if (isContextConsumer(element)) {
      return 'Context.Consumer';
    }
    if (isForwardRef(element)) {
      if (type.displayName) {
        return type.displayName;
      }
      const functionName = type.render.displayName || type.render.name || '';
      return functionName === '' ? 'ForwardRef' : `ForwardRef(${functionName})`;
    }
    if (isMemo(element)) {
      const functionName = type.displayName || type.type.displayName || type.type.name || '';
      return functionName === '' ? 'Memo' : `Memo(${functionName})`;
    }
  }
  return 'UNDEFINED';
};
const getPropKeys = element => {
  const {
    props
  } = element;
  return Object.keys(props).filter(key => key !== 'children' && props[key] !== undefined).sort();
};
const serialize = (element, config, indentation, depth, refs, printer) => ++depth > config.maxDepth ? (0, _markup.printElementAsLeaf)(getType(element), config) : (0, _markup.printElement)(getType(element), (0, _markup.printProps)(getPropKeys(element), element.props, config, indentation + config.indent, depth, refs, printer), (0, _markup.printChildren)(getChildren(element.props.children), config, indentation + config.indent, depth, refs, printer), config, indentation);
exports.serialize = serialize;
const test = val => val != null && isElement(val);
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/ReactTestComponent.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
var _markup = __webpack_require__("./src/plugins/lib/markup.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
// Child can be `number` in Stack renderer but not in Fiber renderer.

const testSymbol = typeof Symbol === 'function' && Symbol.for ? Symbol.for('react.test.json') : 0xe_a7_13_57;
const getPropKeys = object => {
  const {
    props
  } = object;
  return props ? Object.keys(props).filter(key => props[key] !== undefined).sort() : [];
};
const serialize = (object, config, indentation, depth, refs, printer) => ++depth > config.maxDepth ? (0, _markup.printElementAsLeaf)(object.type, config) : (0, _markup.printElement)(object.type, object.props ? (0, _markup.printProps)(getPropKeys(object), object.props, config, indentation + config.indent, depth, refs, printer) : '', object.children ? (0, _markup.printChildren)(object.children, config, indentation + config.indent, depth, refs, printer) : '', config, indentation);
exports.serialize = serialize;
const test = val => val && val.$$typeof === testSymbol;
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins/lib/escapeHTML.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = escapeHTML;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function escapeHTML(str) {
  return str.replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

/***/ },

/***/ "./src/plugins/lib/markup.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printText = exports.printProps = exports.printElementAsLeaf = exports.printElement = exports.printComment = exports.printChildren = void 0;
var _escapeHTML = _interopRequireDefault(__webpack_require__("./src/plugins/lib/escapeHTML.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Return empty string if keys is empty.
const printProps = (keys, props, config, indentation, depth, refs, printer) => {
  const indentationNext = indentation + config.indent;
  const colors = config.colors;
  return keys.map(key => {
    const value = props[key];
    let printed = printer(value, config, indentationNext, depth, refs);
    if (typeof value !== 'string') {
      if (printed.includes('\n')) {
        printed = config.spacingOuter + indentationNext + printed + config.spacingOuter + indentation;
      }
      printed = `{${printed}}`;
    }
    return `${config.spacingInner + indentation + colors.prop.open + key + colors.prop.close}=${colors.value.open}${printed}${colors.value.close}`;
  }).join('');
};

// Return empty string if children is empty.
exports.printProps = printProps;
const printChildren = (children, config, indentation, depth, refs, printer) => children.map(child => config.spacingOuter + indentation + (typeof child === 'string' ? printText(child, config) : printer(child, config, indentation, depth, refs))).join('');
exports.printChildren = printChildren;
const printText = (text, config) => {
  const contentColor = config.colors.content;
  return contentColor.open + (0, _escapeHTML.default)(text) + contentColor.close;
};
exports.printText = printText;
const printComment = (comment, config) => {
  const commentColor = config.colors.comment;
  return `${commentColor.open}<!--${(0, _escapeHTML.default)(comment)}-->${commentColor.close}`;
};

// Separate the functions to format props, children, and element,
// so a plugin could override a particular function, if needed.
// Too bad, so sad: the traditional (but unnecessary) space
// in a self-closing tagColor requires a second test of printedProps.
exports.printComment = printComment;
const printElement = (type, printedProps, printedChildren, config, indentation) => {
  const tagColor = config.colors.tag;
  return `${tagColor.open}<${type}${printedProps && tagColor.close + printedProps + config.spacingOuter + indentation + tagColor.open}${printedChildren ? `>${tagColor.close}${printedChildren}${config.spacingOuter}${indentation}${tagColor.open}</${type}` : `${printedProps && !config.min ? '' : ' '}/`}>${tagColor.close}`;
};
exports.printElement = printElement;
const printElementAsLeaf = (type, config) => {
  const tagColor = config.colors.tag;
  return `${tagColor.open}<${type}${tagColor.close} …${tagColor.open} />${tagColor.close}`;
};
exports.printElementAsLeaf = printElementAsLeaf;

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
exports["default"] = exports.DEFAULT_OPTIONS = void 0;
exports.format = format;
exports.plugins = void 0;
var _ansiStyles = _interopRequireDefault(require("ansi-styles"));
var _collections = __webpack_require__("./src/collections.ts");
var _AsymmetricMatcher = _interopRequireDefault(__webpack_require__("./src/plugins/AsymmetricMatcher.ts"));
var _DOMCollection = _interopRequireDefault(__webpack_require__("./src/plugins/DOMCollection.ts"));
var _DOMElement = _interopRequireDefault(__webpack_require__("./src/plugins/DOMElement.ts"));
var _Immutable = _interopRequireDefault(__webpack_require__("./src/plugins/Immutable.ts"));
var _ReactElement = _interopRequireDefault(__webpack_require__("./src/plugins/ReactElement.ts"));
var _ReactTestComponent = _interopRequireDefault(__webpack_require__("./src/plugins/ReactTestComponent.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const src_toString = Object.prototype.toString;
const toISOString = Date.prototype.toISOString;
const errorToString = Error.prototype.toString;
const regExpToString = RegExp.prototype.toString;

/**
 * Explicitly comparing typeof constructor to function avoids undefined as name
 * when mock identity-obj-proxy returns the key as the value for any key.
 */
const getConstructorName = val => typeof val.constructor === 'function' && val.constructor.name || 'Object';

/* global window */
/** Is val is equal to global window object? Works even if it does not exist :) */
const isWindow = val =>
// eslint-disable-next-line unicorn/prefer-global-this
typeof window !== 'undefined' && val === window;
const SYMBOL_REGEXP = /^Symbol\((.*)\)(.*)$/;
const NEWLINE_REGEXP = /\n/gi;
class PrettyFormatPluginError extends Error {
  constructor(message, stack) {
    super(message);
    this.stack = stack;
    this.name = this.constructor.name;
  }
}
function isToStringedArrayType(toStringed) {
  return toStringed === '[object Array]' || toStringed === '[object ArrayBuffer]' || toStringed === '[object DataView]' || toStringed === '[object Float32Array]' || toStringed === '[object Float64Array]' || toStringed === '[object Int8Array]' || toStringed === '[object Int16Array]' || toStringed === '[object Int32Array]' || toStringed === '[object Uint8Array]' || toStringed === '[object Uint8ClampedArray]' || toStringed === '[object Uint16Array]' || toStringed === '[object Uint32Array]';
}
function printNumber(val) {
  return Object.is(val, -0) ? '-0' : String(val);
}
function printBigInt(val) {
  return String(`${val}n`);
}
function printFunction(val, printFunctionName) {
  if (!printFunctionName) {
    return '[Function]';
  }
  return `[Function ${val.name || 'anonymous'}]`;
}
function printSymbol(val) {
  return String(val).replace(SYMBOL_REGEXP, 'Symbol($1)');
}
function printError(val) {
  return `[${errorToString.call(val)}]`;
}

/**
 * The first port of call for printing an object, handles most of the
 * data-types in JS.
 */
function printBasicValue(val, printFunctionName, escapeRegex, escapeString) {
  if (val === true || val === false) {
    return `${val}`;
  }
  if (val === undefined) {
    return 'undefined';
  }
  if (val === null) {
    return 'null';
  }
  const typeOf = typeof val;
  if (typeOf === 'number') {
    return printNumber(val);
  }
  if (typeOf === 'bigint') {
    return printBigInt(val);
  }
  if (typeOf === 'string') {
    if (escapeString) {
      return `"${val.replaceAll(/"|\\/g, '\\$&')}"`;
    }
    return `"${val}"`;
  }
  if (typeOf === 'function') {
    return printFunction(val, printFunctionName);
  }
  if (typeOf === 'symbol') {
    return printSymbol(val);
  }
  const toStringed = src_toString.call(val);
  if (toStringed === '[object Promise]') {
    return 'Promise {}';
  }
  if (toStringed === '[object WeakMap]') {
    return 'WeakMap {}';
  }
  if (toStringed === '[object WeakSet]') {
    return 'WeakSet {}';
  }
  if (toStringed === '[object Function]' || toStringed === '[object GeneratorFunction]') {
    return printFunction(val, printFunctionName);
  }
  if (toStringed === '[object Symbol]') {
    return printSymbol(val);
  }
  if (toStringed === '[object Date]') {
    return Number.isNaN(+val) ? 'Date { NaN }' : toISOString.call(val);
  }
  if (toStringed === '[object Error]') {
    return printError(val);
  }
  if (toStringed === '[object RegExp]') {
    if (escapeRegex) {
      // https://github.com/benjamingr/RegExp.escape/blob/main/polyfill.js
      return regExpToString.call(val).replaceAll(/[$()*+.?[\\\]^{|}]/g, '\\$&');
    }
    return regExpToString.call(val);
  }
  if (val instanceof Error) {
    return printError(val);
  }
  return null;
}

/**
 * Handles more complex objects (such as objects with circular references,
 * maps and sets etc.)
 */
function printComplexValue(val, config, indentation, depth, refs, hasCalledToJSON) {
  if (refs.includes(val)) {
    return '[Circular]';
  }
  refs = [...refs, val];
  const hitMaxDepth = ++depth > config.maxDepth;
  const min = config.min;
  if (config.callToJSON && !hitMaxDepth && val.toJSON && typeof val.toJSON === 'function' && !hasCalledToJSON) {
    return printer(val.toJSON(), config, indentation, depth, refs, true);
  }
  const toStringed = src_toString.call(val);
  if (toStringed === '[object Arguments]') {
    return hitMaxDepth ? '[Arguments]' : `${min ? '' : 'Arguments '}[${(0, _collections.printListItems)(val, config, indentation, depth, refs, printer)}]`;
  }
  if (isToStringedArrayType(toStringed)) {
    return hitMaxDepth ? `[${val.constructor.name}]` : `${min ? '' : !config.printBasicPrototype && val.constructor.name === 'Array' ? '' : `${val.constructor.name} `}[${(0, _collections.printListItems)(val, config, indentation, depth, refs, printer)}]`;
  }
  if (toStringed === '[object Map]') {
    return hitMaxDepth ? '[Map]' : `Map {${(0, _collections.printIteratorEntries)(val.entries(), config, indentation, depth, refs, printer, ' => ')}}`;
  }
  if (toStringed === '[object Set]') {
    return hitMaxDepth ? '[Set]' : `Set {${(0, _collections.printIteratorValues)(val.values(), config, indentation, depth, refs, printer)}}`;
  }

  // Avoid failure to serialize global window object in jsdom test environment.
  // For example, not even relevant if window is prop of React element.
  return hitMaxDepth || isWindow(val) ? `[${getConstructorName(val)}]` : `${min ? '' : !config.printBasicPrototype && getConstructorName(val) === 'Object' ? '' : `${getConstructorName(val)} `}{${(0, _collections.printObjectProperties)(val, config, indentation, depth, refs, printer)}}`;
}
function isNewPlugin(plugin) {
  return plugin.serialize != null;
}
function printPlugin(plugin, val, config, indentation, depth, refs) {
  let printed;
  try {
    printed = isNewPlugin(plugin) ? plugin.serialize(val, config, indentation, depth, refs, printer) : plugin.print(val, valChild => printer(valChild, config, indentation, depth, refs), str => {
      const indentationNext = indentation + config.indent;
      return indentationNext + str.replaceAll(NEWLINE_REGEXP, `\n${indentationNext}`);
    }, {
      edgeSpacing: config.spacingOuter,
      min: config.min,
      spacing: config.spacingInner
    }, config.colors);
  } catch (error) {
    throw new PrettyFormatPluginError(error.message, error.stack);
  }
  if (typeof printed !== 'string') {
    throw new TypeError(`pretty-format: Plugin must return type "string" but instead returned "${typeof printed}".`);
  }
  return printed;
}
function findPlugin(plugins, val) {
  for (const plugin of plugins) {
    try {
      if (plugin.test(val)) {
        return plugin;
      }
    } catch (error) {
      throw new PrettyFormatPluginError(error.message, error.stack);
    }
  }
  return null;
}
function printer(val, config, indentation, depth, refs, hasCalledToJSON) {
  const plugin = findPlugin(config.plugins, val);
  if (plugin !== null) {
    return printPlugin(plugin, val, config, indentation, depth, refs);
  }
  const basicResult = printBasicValue(val, config.printFunctionName, config.escapeRegex, config.escapeString);
  if (basicResult !== null) {
    return basicResult;
  }
  return printComplexValue(val, config, indentation, depth, refs, hasCalledToJSON);
}
const DEFAULT_THEME = {
  comment: 'gray',
  content: 'reset',
  prop: 'yellow',
  tag: 'cyan',
  value: 'green'
};
const DEFAULT_THEME_KEYS = Object.keys(DEFAULT_THEME);

// could be replaced by `satisfies` operator in the future: https://github.com/microsoft/TypeScript/issues/47920
const toOptionsSubtype = options => options;
const DEFAULT_OPTIONS = exports.DEFAULT_OPTIONS = toOptionsSubtype({
  callToJSON: true,
  compareKeys: undefined,
  escapeRegex: false,
  escapeString: true,
  highlight: false,
  indent: 2,
  maxDepth: Number.POSITIVE_INFINITY,
  maxWidth: Number.POSITIVE_INFINITY,
  min: false,
  plugins: [],
  printBasicPrototype: true,
  printFunctionName: true,
  theme: DEFAULT_THEME
});
function validateOptions(options) {
  for (const key of Object.keys(options)) {
    if (!Object.prototype.hasOwnProperty.call(DEFAULT_OPTIONS, key)) {
      throw new Error(`pretty-format: Unknown option "${key}".`);
    }
  }
  if (options.min && options.indent !== undefined && options.indent !== 0) {
    throw new Error('pretty-format: Options "min" and "indent" cannot be used together.');
  }
  if (options.theme !== undefined) {
    if (options.theme === null) {
      throw new Error('pretty-format: Option "theme" must not be null.');
    }
    if (typeof options.theme !== 'object') {
      throw new TypeError(`pretty-format: Option "theme" must be of type "object" but instead received "${typeof options.theme}".`);
    }
  }
}
const getColorsHighlight = options => DEFAULT_THEME_KEYS.reduce((colors, key) => {
  const value = options.theme && options.theme[key] !== undefined ? options.theme[key] : DEFAULT_THEME[key];
  const color = value && _ansiStyles.default[value];
  if (color && typeof color.close === 'string' && typeof color.open === 'string') {
    colors[key] = color;
  } else {
    throw new Error(`pretty-format: Option "theme" has a key "${key}" whose value "${value}" is undefined in ansi-styles.`);
  }
  return colors;
}, Object.create(null));
const getColorsEmpty = () => DEFAULT_THEME_KEYS.reduce((colors, key) => {
  colors[key] = {
    close: '',
    open: ''
  };
  return colors;
}, Object.create(null));
const getPrintFunctionName = options => options?.printFunctionName ?? DEFAULT_OPTIONS.printFunctionName;
const getEscapeRegex = options => options?.escapeRegex ?? DEFAULT_OPTIONS.escapeRegex;
const getEscapeString = options => options?.escapeString ?? DEFAULT_OPTIONS.escapeString;
const getConfig = options => ({
  callToJSON: options?.callToJSON ?? DEFAULT_OPTIONS.callToJSON,
  colors: options?.highlight ? getColorsHighlight(options) : getColorsEmpty(),
  compareKeys: typeof options?.compareKeys === 'function' || options?.compareKeys === null ? options.compareKeys : DEFAULT_OPTIONS.compareKeys,
  escapeRegex: getEscapeRegex(options),
  escapeString: getEscapeString(options),
  indent: options?.min ? '' : createIndent(options?.indent ?? DEFAULT_OPTIONS.indent),
  maxDepth: options?.maxDepth ?? DEFAULT_OPTIONS.maxDepth,
  maxWidth: options?.maxWidth ?? DEFAULT_OPTIONS.maxWidth,
  min: options?.min ?? DEFAULT_OPTIONS.min,
  plugins: options?.plugins ?? DEFAULT_OPTIONS.plugins,
  printBasicPrototype: options?.printBasicPrototype ?? true,
  printFunctionName: getPrintFunctionName(options),
  spacingInner: options?.min ? ' ' : '\n',
  spacingOuter: options?.min ? '' : '\n'
});
function createIndent(indent) {
  return Array.from({
    length: indent + 1
  }).join(' ');
}

/**
 * Returns a presentation string of your `val` object
 * @param val any potential JavaScript object
 * @param options Custom settings
 */
function format(val, options) {
  if (options) {
    validateOptions(options);
    if (options.plugins) {
      const plugin = findPlugin(options.plugins, val);
      if (plugin !== null) {
        return printPlugin(plugin, val, getConfig(options), '', 0, []);
      }
    }
  }
  const basicResult = printBasicValue(val, getPrintFunctionName(options), getEscapeRegex(options), getEscapeString(options));
  if (basicResult !== null) {
    return basicResult;
  }
  return printComplexValue(val, getConfig(options), '', 0, []);
}
const plugins = exports.plugins = {
  AsymmetricMatcher: _AsymmetricMatcher.default,
  DOMCollection: _DOMCollection.default,
  DOMElement: _DOMElement.default,
  Immutable: _Immutable.default,
  ReactElement: _ReactElement.default,
  ReactTestComponent: _ReactTestComponent.default
};
var _default = exports["default"] = format;
})();

module.exports = __webpack_exports__;
/******/ })()
;