"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = normalizeFile;
function _debug() {
  const data = require("debug");
  _debug = function () {
    return data;
  };
  return data;
}
function _t() {
  const data = require("@babel/types");
  _t = function () {
    return data;
  };
  return data;
}
function _convertSourceMap() {
  const data = require("convert-source-map");
  _convertSourceMap = function () {
    return data;
  };
  return data;
}
var _readInputSourceMapFile = require("./read-input-source-map-file.js");
var _file = require("./file/file.js");
var _index = require("../parser/index.js");
var _cloneDeep = require("./util/clone-deep.js");
const {
  file,
  traverseFast
} = _t();
const debug = _debug()("babel:transform:file");
const INLINE_SOURCEMAP_REGEX = /^[@#]\s+sourceMappingURL=data:(?:application|text)\/json;(?:charset[:=]\S+?;)?base64,.*$/;
const EXTERNAL_SOURCEMAP_REGEX = /^[@#][ \t]+sourceMappingURL=([^\s'"`]+)[ \t]*$/;
function* normalizeFile(pluginPasses, options, code, ast) {
  code = `${code || ""}`;
  if (ast) {
    if (ast.type === "Program") {
      ast = file(ast, [], []);
    } else if (ast.type !== "File") {
      throw new Error("AST root must be a Program or File node");
    }
    if (options.cloneInputAst) {
      ast = (0, _cloneDeep.default)(ast);
    }
  } else {
    ast = yield* (0, _index.default)(pluginPasses, options, code);
  }
  let inputMap = null;
  if (options.inputSourceMap !== false) {
    if (typeof options.inputSourceMap === "object") {
      inputMap = _convertSourceMap().fromObject(options.inputSourceMap);
    }
    if (!inputMap) {
      const lastComment = extractComments(INLINE_SOURCEMAP_REGEX, ast);
      if (lastComment) {
        try {
          inputMap = _convertSourceMap().fromComment("//" + lastComment);
        } catch (err) {
          debug("discarding unknown inline input sourcemap");
        }
      }
    }
    if (!inputMap) {
      const lastComment = extractComments(EXTERNAL_SOURCEMAP_REGEX, ast);
      if (typeof options.filename === "string" && lastComment) {
        try {
          const inputMapURL = EXTERNAL_SOURCEMAP_REGEX.exec(lastComment)[1];
          inputMap = (0, _readInputSourceMapFile.default)(options.filename, options.root, inputMapURL);
        } catch (err) {
          debug("discarding unknown file input sourcemap", err);
        }
      } else if (lastComment) {
        debug("discarding un-loadable file input sourcemap");
      }
    }
  }
  return new _file.default(options, {
    code,
    ast: ast,
    inputMap
  });
}
function extractCommentsFromList(regex, comments, lastComment) {
  if (comments) {
    comments = comments.filter(({
      value
    }) => {
      if (regex.test(value)) {
        lastComment = value;
        return false;
      }
      return true;
    });
  }
  return [comments, lastComment];
}
function extractComments(regex, ast) {
  let lastComment = null;
  traverseFast(ast, node => {
    [node.leadingComments, lastComment] = extractCommentsFromList(regex, node.leadingComments, lastComment);
    [node.innerComments, lastComment] = extractCommentsFromList(regex, node.innerComments, lastComment);
    [node.trailingComments, lastComment] = extractCommentsFromList(regex, node.trailingComments, lastComment);
  });
  return lastComment;
}
0 && 0;

//# sourceMappingURL=normalize-file.js.map
