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
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.DependencyResolver = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _jestSnapshot() {
  const data = require("jest-snapshot");
  _jestSnapshot = function () {
    return data;
  };
  return data;
}
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/**
 * DependencyResolver is used to resolve the direct dependencies of a module or
 * to retrieve a list of all transitive inverse dependencies.
 */
class DependencyResolver {
  _hasteFS;
  _resolver;
  _snapshotResolver;
  constructor(resolver, hasteFS, snapshotResolver) {
    this._resolver = resolver;
    this._hasteFS = hasteFS;
    this._snapshotResolver = snapshotResolver;
  }
  resolve(file, options) {
    const dependencies = this._hasteFS.getDependencies(file);
    const fallbackOptions = {
      conditions: undefined
    };
    if (!dependencies) {
      return [];
    }
    return dependencies.reduce((acc, dependency) => {
      if (this._resolver.isCoreModule(dependency)) {
        return acc;
      }
      let resolvedDependency;
      let resolvedMockDependency;
      try {
        resolvedDependency = this._resolver.resolveModule(file, dependency, options ?? fallbackOptions);
      } catch {
        try {
          resolvedDependency = this._resolver.getMockModule(file, dependency, options ?? fallbackOptions);
        } catch {
          // leave resolvedDependency as undefined if nothing can be found
        }
      }
      if (resolvedDependency == null) {
        return acc;
      }
      acc.push(resolvedDependency);

      // If we resolve a dependency, then look for a mock dependency
      // of the same name in that dependency's directory.
      try {
        resolvedMockDependency = this._resolver.getMockModule(resolvedDependency, path().basename(dependency), options ?? fallbackOptions);
      } catch {
        // leave resolvedMockDependency as undefined if nothing can be found
      }
      if (resolvedMockDependency != null) {
        const dependencyMockDir = path().resolve(path().dirname(resolvedDependency), '__mocks__');
        resolvedMockDependency = path().resolve(resolvedMockDependency);

        // make sure mock is in the correct directory
        if (dependencyMockDir === path().dirname(resolvedMockDependency)) {
          acc.push(resolvedMockDependency);
        }
      }
      return acc;
    }, []);
  }
  resolveInverseModuleMap(paths, filter, options) {
    if (paths.size === 0) {
      return [];
    }
    const collectModules = (related, moduleMap, changed) => {
      const visitedModules = new Set();
      const result = [];
      while (changed.size > 0) {
        changed = new Set(moduleMap.reduce((acc, module) => {
          if (visitedModules.has(module.file) || !module.dependencies.some(dep => changed.has(dep))) {
            return acc;
          }
          const file = module.file;
          if (filter(file)) {
            result.push(module);
            related.delete(file);
          }
          visitedModules.add(file);
          acc.push(file);
          return acc;
        }, []));
      }
      return [...result, ...[...related].map(file => ({
        dependencies: [],
        file
      }))];
    };
    const relatedPaths = new Set();
    const changed = new Set();
    for (const path of paths) {
      if (this._hasteFS.exists(path)) {
        const modulePath = (0, _jestSnapshot().isSnapshotPath)(path) ? this._snapshotResolver.resolveTestPath(path) : path;
        changed.add(modulePath);
        if (filter(modulePath)) {
          relatedPaths.add(modulePath);
        }
      }
    }
    const modules = [];
    for (const file of this._hasteFS.getAbsoluteFileIterator()) {
      modules.push({
        dependencies: this.resolve(file, options),
        file
      });
    }
    return collectModules(relatedPaths, modules, changed);
  }
  resolveInverse(paths, filter, options) {
    return this.resolveInverseModuleMap(paths, filter, options).map(module => module.file);
  }
}
exports.DependencyResolver = DependencyResolver;
})();

module.exports = __webpack_exports__;
/******/ })()
;