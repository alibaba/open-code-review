"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Kind = exports.Hint = exports.OptionalKind = exports.ReadonlyKind = exports.TransformKind = void 0;
/** Symbol key applied to transform types */
exports.TransformKind = Symbol.for('TypeBox.Transform');
/** Symbol key applied to readonly types */
exports.ReadonlyKind = Symbol.for('TypeBox.Readonly');
/** Symbol key applied to optional types */
exports.OptionalKind = Symbol.for('TypeBox.Optional');
/** Symbol key applied to types */
exports.Hint = Symbol.for('TypeBox.Hint');
/** Symbol key applied to types */
exports.Kind = Symbol.for('TypeBox.Kind');
