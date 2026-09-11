"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Module = void 0;
const parse_1 = require("./parse");
// ------------------------------------------------------------------
// Module
// ------------------------------------------------------------------
class Module {
    constructor(properties) {
        this.properties = properties;
    }
    /** Parses using one of the parsers defined on this instance */
    Parse(...args) {
        // prettier-ignore
        const [key, content, context] = (args.length === 3 ? [args[0], args[1], args[2]] :
            args.length === 2 ? [args[0], args[1], undefined] :
                (() => { throw Error('Invalid parse arguments'); })());
        return (0, parse_1.Parse)(this.properties, this.properties[key], content, context);
    }
}
exports.Module = Module;
