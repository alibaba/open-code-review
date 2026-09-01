"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.default = readInputSourceMapFile;
function _fs() {
  const data = require("fs");
  _fs = function () {
    return data;
  };
  return data;
}
function _path() {
  const data = require("path");
  _path = function () {
    return data;
  };
  return data;
}
function _debug() {
  const data = require("debug");
  _debug = function () {
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
const debug = _debug()("babel:transform:file");
function findUpSync(name, {
  cwd,
  stopAt
} = {}) {
  let directory = _path().resolve(cwd || "");
  const {
    root
  } = _path().parse(directory);
  stopAt = _path().resolve(directory, stopAt || root);
  const isAbsoluteName = _path().isAbsolute(name);
  while (directory) {
    const filePath = isAbsoluteName ? name : _path().join(directory, name);
    try {
      const stats = _fs().statSync(filePath);
      if (stats.isFile()) {
        return filePath;
      }
    } catch (_) {}
    if (directory === stopAt || directory === root) {
      break;
    }
    directory = _path().dirname(directory);
  }
}
function getInputMapPath(filename, root, inputMapURL) {
  const inputFileDir = _path().dirname(filename);
  const inputMapPath = _path().resolve(inputFileDir, inputMapURL);
  const relativeToInputFileDir = _path().relative(inputFileDir, inputMapPath);
  if (relativeToInputFileDir.startsWith("..") || _path().isAbsolute(relativeToInputFileDir)) {
    const inputPackageJSONPath = findUpSync("package.json", {
      cwd: inputFileDir,
      stopAt: root
    });
    const inputFileRoot = inputPackageJSONPath ? _path().dirname(inputPackageJSONPath) : root;
    const relativeInputMapPath = _path().relative(inputFileRoot, inputMapPath);
    if (relativeInputMapPath.startsWith("..") || _path().isAbsolute(relativeInputMapPath)) {
      debug(`discarding input sourcemap "${inputMapPath}" outside of package root "${inputFileRoot}"`);
      return null;
    }
  }
  return inputMapPath;
}
function readInputSourceMapFile(filename, root, inputMapURL) {
  const inputMapPath = getInputMapPath(filename, root, inputMapURL);
  if (inputMapPath) {
    const inputMapContent = _fs().readFileSync(inputMapPath, "utf8");
    return _convertSourceMap().fromJSON(inputMapContent);
  }
  return null;
}
0 && 0;

//# sourceMappingURL=read-input-source-map-file.js.map
