"use strict";

var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __exportStar = (this && this.__exportStar) || function(m, exports) {
    for (var p in m) if (p !== "default" && !Object.prototype.hasOwnProperty.call(exports, p)) __createBinding(exports, m, p);
};
Object.defineProperty(exports, "__esModule", { value: true });
// ------------------------------------------------------------------
// Infrastructure
// ------------------------------------------------------------------
__exportStar(require("./type/clone/index"), exports);
__exportStar(require("./type/create/index"), exports);
__exportStar(require("./type/error/index"), exports);
__exportStar(require("./type/guard/index"), exports);
__exportStar(require("./type/helpers/index"), exports);
__exportStar(require("./type/patterns/index"), exports);
__exportStar(require("./type/registry/index"), exports);
__exportStar(require("./type/sets/index"), exports);
__exportStar(require("./type/symbols/index"), exports);
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
__exportStar(require("./type/any/index"), exports);
__exportStar(require("./type/array/index"), exports);
__exportStar(require("./type/argument/index"), exports);
__exportStar(require("./type/async-iterator/index"), exports);
__exportStar(require("./type/awaited/index"), exports);
__exportStar(require("./type/bigint/index"), exports);
__exportStar(require("./type/boolean/index"), exports);
__exportStar(require("./type/composite/index"), exports);
__exportStar(require("./type/const/index"), exports);
__exportStar(require("./type/constructor/index"), exports);
__exportStar(require("./type/constructor-parameters/index"), exports);
__exportStar(require("./type/date/index"), exports);
__exportStar(require("./type/enum/index"), exports);
__exportStar(require("./type/exclude/index"), exports);
__exportStar(require("./type/extends/index"), exports);
__exportStar(require("./type/extract/index"), exports);
__exportStar(require("./type/function/index"), exports);
__exportStar(require("./type/indexed/index"), exports);
__exportStar(require("./type/instance-type/index"), exports);
__exportStar(require("./type/instantiate/index"), exports);
__exportStar(require("./type/integer/index"), exports);
__exportStar(require("./type/intersect/index"), exports);
__exportStar(require("./type/iterator/index"), exports);
__exportStar(require("./type/intrinsic/index"), exports);
__exportStar(require("./type/keyof/index"), exports);
__exportStar(require("./type/literal/index"), exports);
__exportStar(require("./type/module/index"), exports);
__exportStar(require("./type/mapped/index"), exports);
__exportStar(require("./type/never/index"), exports);
__exportStar(require("./type/not/index"), exports);
__exportStar(require("./type/null/index"), exports);
__exportStar(require("./type/number/index"), exports);
__exportStar(require("./type/object/index"), exports);
__exportStar(require("./type/omit/index"), exports);
__exportStar(require("./type/optional/index"), exports);
__exportStar(require("./type/parameters/index"), exports);
__exportStar(require("./type/partial/index"), exports);
__exportStar(require("./type/pick/index"), exports);
__exportStar(require("./type/promise/index"), exports);
__exportStar(require("./type/readonly/index"), exports);
__exportStar(require("./type/readonly-optional/index"), exports);
__exportStar(require("./type/record/index"), exports);
__exportStar(require("./type/recursive/index"), exports);
__exportStar(require("./type/ref/index"), exports);
__exportStar(require("./type/regexp/index"), exports);
__exportStar(require("./type/required/index"), exports);
__exportStar(require("./type/rest/index"), exports);
__exportStar(require("./type/return-type/index"), exports);
__exportStar(require("./type/schema/index"), exports);
__exportStar(require("./type/static/index"), exports);
__exportStar(require("./type/string/index"), exports);
__exportStar(require("./type/symbol/index"), exports);
__exportStar(require("./type/template-literal/index"), exports);
__exportStar(require("./type/transform/index"), exports);
__exportStar(require("./type/tuple/index"), exports);
__exportStar(require("./type/uint8array/index"), exports);
__exportStar(require("./type/undefined/index"), exports);
__exportStar(require("./type/union/index"), exports);
__exportStar(require("./type/unknown/index"), exports);
__exportStar(require("./type/unsafe/index"), exports);
__exportStar(require("./type/void/index"), exports);
// ------------------------------------------------------------------
// Type.*
// ------------------------------------------------------------------
__exportStar(require("./type/type/index"), exports);
