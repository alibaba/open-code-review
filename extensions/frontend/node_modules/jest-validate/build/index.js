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

/***/ "./src/condition.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getValues = getValues;
exports.multipleValidOptions = multipleValidOptions;
exports.validationCondition = validationCondition;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const toString = Object.prototype.toString;
const MULTIPLE_VALID_OPTIONS_SYMBOL = Symbol('JEST_MULTIPLE_VALID_OPTIONS');
function validationConditionSingle(option, validOption) {
  return option === null || option === undefined || typeof option === 'function' && typeof validOption === 'function' || toString.call(option) === toString.call(validOption);
}
function getValues(validOption) {
  if (Array.isArray(validOption) &&
  // @ts-expect-error: no index signature
  validOption[MULTIPLE_VALID_OPTIONS_SYMBOL]) {
    return validOption;
  }
  return [validOption];
}
function validationCondition(option, validOption) {
  return getValues(validOption).some(e => validationConditionSingle(option, e));
}
function multipleValidOptions(...args) {
  const options = [...args];
  // @ts-expect-error: no index signature
  options[MULTIPLE_VALID_OPTIONS_SYMBOL] = true;
  return options;
}

/***/ },

/***/ "./src/defaultConfig.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _condition = __webpack_require__("./src/condition.ts");
var _deprecated = __webpack_require__("./src/deprecated.ts");
var _errors = __webpack_require__("./src/errors.ts");
var _utils = __webpack_require__("./src/utils.ts");
var _warnings = __webpack_require__("./src/warnings.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const validationOptions = {
  comment: '',
  condition: _condition.validationCondition,
  deprecate: _deprecated.deprecationWarning,
  deprecatedConfig: {},
  error: _errors.errorMessage,
  exampleConfig: {},
  recursive: true,
  // Allow NPM-sanctioned comments in package.json. Use a "//" key.
  recursiveDenylist: ['//'],
  title: {
    deprecation: _utils.DEPRECATION,
    error: _utils.ERROR,
    warning: _utils.WARNING
  },
  unknown: _warnings.unknownOptionWarning
};
var _default = exports["default"] = validationOptions;

/***/ },

/***/ "./src/deprecated.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.deprecationWarning = void 0;
var _utils = __webpack_require__("./src/utils.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const deprecationMessage = (message, options) => {
  const comment = options.comment;
  const name = options.title && options.title.deprecation || _utils.DEPRECATION;
  (0, _utils.logValidationWarning)(name, message, comment);
};
const deprecationWarning = (config, option, deprecatedOptions, options) => {
  if (option in deprecatedOptions) {
    deprecationMessage(deprecatedOptions[option](config), options);
    return true;
  }
  return false;
};
exports.deprecationWarning = deprecationWarning;

/***/ },

/***/ "./src/errors.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.errorMessage = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _getType() {
  const data = require("@jest/get-type");
  _getType = function () {
    return data;
  };
  return data;
}
var _condition = __webpack_require__("./src/condition.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const errorMessage = (option, received, defaultValue, options, path) => {
  const conditions = (0, _condition.getValues)(defaultValue);
  const validTypes = [...new Set(conditions.map(_getType().getType))];
  const message = `  Option ${_chalk().default.bold(`"${path && path.length > 0 ? `${path.join('.')}.` : ''}${option}"`)} must be of type:
    ${validTypes.map(e => _chalk().default.bold.green(e)).join(' or ')}
  but instead received:
    ${_chalk().default.bold.red((0, _getType().getType)(received))}

  Example:
${formatExamples(option, conditions)}`;
  const comment = options.comment;
  const name = options.title && options.title.error || _utils.ERROR;
  throw new _utils.ValidationError(name, message, comment);
};
exports.errorMessage = errorMessage;
function formatExamples(option, examples) {
  return examples.map(e => `  {
    ${_chalk().default.bold(`"${option}"`)}: ${_chalk().default.bold((0, _utils.formatPrettyObject)(e))}
  }`).join(`

  or

`);
}

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.logValidationWarning = exports.formatPrettyObject = exports.format = exports.createDidYouMeanMessage = exports.WARNING = exports.ValidationError = exports.ERROR = exports.DEPRECATION = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _leven() {
  const data = _interopRequireDefault(require("leven"));
  _leven = function () {
    return data;
  };
  return data;
}
function _prettyFormat() {
  const data = require("pretty-format");
  _prettyFormat = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BULLET = _chalk().default.bold('\u25CF');
const DEPRECATION = exports.DEPRECATION = `${BULLET} Deprecation Warning`;
const ERROR = exports.ERROR = `${BULLET} Validation Error`;
const WARNING = exports.WARNING = `${BULLET} Validation Warning`;
const format = value => typeof value === 'function' ? value.toString() : (0, _prettyFormat().format)(value, {
  min: true
});
exports.format = format;
const formatPrettyObject = value => typeof value === 'function' ? value.toString() : value === undefined ? 'undefined' : JSON.stringify(value, null, 2).split('\n').join('\n    ');
exports.formatPrettyObject = formatPrettyObject;
class ValidationError extends Error {
  name;
  message;
  constructor(name, message, comment) {
    super();
    comment = comment ? `\n\n${comment}` : '\n';
    this.name = '';
    this.message = _chalk().default.red(`${_chalk().default.bold(name)}:\n\n${message}${comment}`);
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    Error.captureStackTrace(this, () => {});
  }
}
exports.ValidationError = ValidationError;
const logValidationWarning = (name, message, comment) => {
  comment = comment ? `\n\n${comment}` : '\n';
  console.warn(_chalk().default.yellow(`${_chalk().default.bold(name)}:\n\n${message}${comment}`));
};
exports.logValidationWarning = logValidationWarning;
const createDidYouMeanMessage = (unrecognized, allowedOptions) => {
  const suggestion = allowedOptions.find(option => {
    const steps = (0, _leven().default)(option, unrecognized);
    return steps < 3;
  });
  return suggestion ? `Did you mean ${_chalk().default.bold(format(suggestion))}?` : '';
};
exports.createDidYouMeanMessage = createDidYouMeanMessage;

/***/ },

/***/ "./src/validate.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _defaultConfig = _interopRequireDefault(__webpack_require__("./src/defaultConfig.ts"));
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

let hasDeprecationWarnings = false;
const shouldSkipValidationForPath = (path, key, denylist) => denylist ? denylist.includes([...path, key].join('.')) : false;
const _validate = (config, exampleConfig, options, path = []) => {
  if (typeof config !== 'object' || config == null || typeof exampleConfig !== 'object' || exampleConfig == null) {
    return {
      hasDeprecationWarnings
    };
  }
  for (const key in config) {
    if (options.deprecatedConfig && key in options.deprecatedConfig && typeof options.deprecate === 'function') {
      const isDeprecatedKey = options.deprecate(config, key, options.deprecatedConfig, options);
      hasDeprecationWarnings = hasDeprecationWarnings || isDeprecatedKey;
    } else if (allowsMultipleTypes(key)) {
      const value = config[key];
      if (typeof options.condition === 'function' && typeof options.error === 'function') {
        if (key === 'maxWorkers' && !isOfTypeStringOrNumber(value)) {
          throw new _utils.ValidationError('Validation Error', `${key} has to be of type string or number`, 'maxWorkers=50% or\nmaxWorkers=3');
        }
      }
    } else if (Object.hasOwnProperty.call(exampleConfig, key)) {
      if (typeof options.condition === 'function' && typeof options.error === 'function' && !options.condition(config[key], exampleConfig[key])) {
        options.error(key, config[key], exampleConfig[key], options, path);
      }
    } else if (shouldSkipValidationForPath(path, key, options.recursiveDenylist)) {
      // skip validating unknown options inside blacklisted paths
    } else {
      options.unknown?.(config, exampleConfig, key, options, path);
    }
    if (options.recursive && !Array.isArray(exampleConfig[key]) && options.recursiveDenylist && !shouldSkipValidationForPath(path, key, options.recursiveDenylist)) {
      _validate(config[key], exampleConfig[key], options, [...path, key]);
    }
  }
  return {
    hasDeprecationWarnings
  };
};
const allowsMultipleTypes = key => key === 'maxWorkers';
const isOfTypeStringOrNumber = value => typeof value === 'number' || typeof value === 'string';
const validate = (config, options) => {
  hasDeprecationWarnings = false;

  // Preserve default denylist entries even with user-supplied denylist
  const combinedDenylist = [...(_defaultConfig.default.recursiveDenylist || []), ...(options.recursiveDenylist || [])];
  const defaultedOptions = Object.assign({
    ..._defaultConfig.default,
    ...options,
    recursiveDenylist: combinedDenylist,
    title: options.title || _defaultConfig.default.title
  });
  const {
    hasDeprecationWarnings: hdw
  } = _validate(config, options.exampleConfig, defaultedOptions);
  return {
    hasDeprecationWarnings: hdw,
    isValid: true
  };
};
var _default = exports["default"] = validate;

/***/ },

/***/ "./src/validateCLIOptions.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.DOCUMENTATION_NOTE = void 0;
exports["default"] = validateCLIOptions;
function _camelcase() {
  const data = _interopRequireDefault(require("camelcase"));
  _camelcase = function () {
    return data;
  };
  return data;
}
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BULLET = _chalk().default.bold('\u25CF');
const DOCUMENTATION_NOTE = exports.DOCUMENTATION_NOTE = `  ${_chalk().default.bold('CLI Options Documentation:')}
  https://jestjs.io/docs/cli
`;
const createCLIValidationError = (unrecognizedOptions, allowedOptions) => {
  let title = `${BULLET} Unrecognized CLI Parameter`;
  let message;
  const comment = `  ${_chalk().default.bold('CLI Options Documentation')}:\n` + '  https://jestjs.io/docs/cli\n';
  if (unrecognizedOptions.length === 1) {
    const unrecognized = unrecognizedOptions[0];
    const didYouMeanMessage = unrecognized.length > 1 ? (0, _utils.createDidYouMeanMessage)(unrecognized, [...allowedOptions]) : '';
    message = `  Unrecognized option ${_chalk().default.bold((0, _utils.format)(unrecognized))}.${didYouMeanMessage ? ` ${didYouMeanMessage}` : ''}`;
  } else {
    title += 's';
    message = '  Following options were not recognized:\n' + `  ${_chalk().default.bold((0, _utils.format)(unrecognizedOptions))}`;
  }
  return new _utils.ValidationError(title, message, comment);
};
const validateDeprecatedOptions = (deprecatedOptions, deprecationEntries, argv) => {
  for (const opt of deprecatedOptions) {
    const name = opt.name;
    const message = deprecationEntries[name](argv);
    const comment = DOCUMENTATION_NOTE;
    if (opt.fatal) {
      throw new _utils.ValidationError(name, message, comment);
    } else {
      (0, _utils.logValidationWarning)(name, message, comment);
    }
  }
};
function validateCLIOptions(argv, options = {}, rawArgv = []) {
  const yargsSpecialOptions = ['$0', '_', 'help', 'h'];
  const allowedOptions = Object.keys(options).reduce((acc, option) => acc.add(option).add(options[option].alias || option), new Set(yargsSpecialOptions));
  const deprecationEntries = options.deprecationEntries ?? {};
  const CLIDeprecations = Object.keys(deprecationEntries).reduce((acc, entry) => {
    acc[entry] = deprecationEntries[entry];
    if (options[entry]) {
      const alias = options[entry].alias;
      if (alias) {
        acc[alias] = deprecationEntries[entry];
      }
    }
    return acc;
  }, {});
  const deprecations = new Set(Object.keys(CLIDeprecations));
  const deprecatedOptions = Object.keys(argv).filter(arg => deprecations.has(arg) && argv[arg] != null).map(arg => ({
    fatal: !allowedOptions.has(arg),
    name: arg
  }));
  if (deprecatedOptions.length > 0) {
    validateDeprecatedOptions(deprecatedOptions, CLIDeprecations, argv);
  }
  const unrecognizedOptions = Object.keys(argv).filter(arg => !allowedOptions.has((0, _camelcase().default)(arg, {
    locale: 'en-US'
  })) && !allowedOptions.has(arg) && (rawArgv.length === 0 || rawArgv.includes(arg)));
  if (unrecognizedOptions.length > 0) {
    throw createCLIValidationError(unrecognizedOptions, allowedOptions);
  }
  return true;
}

/***/ },

/***/ "./src/warnings.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.unknownOptionWarning = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const unknownOptionWarning = (config, exampleConfig, option, options, path) => {
  const didYouMean = (0, _utils.createDidYouMeanMessage)(option, Object.keys(exampleConfig));
  const message = `  Unknown option ${_chalk().default.bold(`"${path && path.length > 0 ? `${path.join('.')}.` : ''}${option}"`)} with value ${_chalk().default.bold((0, _utils.format)(config[option]))} was found.${didYouMean && ` ${didYouMean}`}\n  This is probably a typing mistake. Fixing it will remove this message.`;
  const comment = options.comment;
  const name = options.title && options.title.warning || _utils.WARNING;
  (0, _utils.logValidationWarning)(name, message, comment);
};
exports.unknownOptionWarning = unknownOptionWarning;

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
Object.defineProperty(exports, "ValidationError", ({
  enumerable: true,
  get: function () {
    return _utils.ValidationError;
  }
}));
Object.defineProperty(exports, "createDidYouMeanMessage", ({
  enumerable: true,
  get: function () {
    return _utils.createDidYouMeanMessage;
  }
}));
Object.defineProperty(exports, "format", ({
  enumerable: true,
  get: function () {
    return _utils.format;
  }
}));
Object.defineProperty(exports, "logValidationWarning", ({
  enumerable: true,
  get: function () {
    return _utils.logValidationWarning;
  }
}));
Object.defineProperty(exports, "multipleValidOptions", ({
  enumerable: true,
  get: function () {
    return _condition.multipleValidOptions;
  }
}));
Object.defineProperty(exports, "validate", ({
  enumerable: true,
  get: function () {
    return _validate.default;
  }
}));
Object.defineProperty(exports, "validateCLIOptions", ({
  enumerable: true,
  get: function () {
    return _validateCLIOptions.default;
  }
}));
var _utils = __webpack_require__("./src/utils.ts");
var _validate = _interopRequireDefault(__webpack_require__("./src/validate.ts"));
var _validateCLIOptions = _interopRequireDefault(__webpack_require__("./src/validateCLIOptions.ts"));
var _condition = __webpack_require__("./src/condition.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;