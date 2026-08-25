"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.PatternNeverExact = exports.PatternStringExact = exports.PatternNumberExact = exports.PatternBooleanExact = exports.PatternNever = exports.PatternString = exports.PatternNumber = exports.PatternBoolean = void 0;
exports.PatternBoolean = '(true|false)';
exports.PatternNumber = '(0|[1-9][0-9]*)';
exports.PatternString = '(.*)';
exports.PatternNever = '(?!.*)';
exports.PatternBooleanExact = `^${exports.PatternBoolean}$`;
exports.PatternNumberExact = `^${exports.PatternNumber}$`;
exports.PatternStringExact = `^${exports.PatternString}$`;
exports.PatternNeverExact = `^${exports.PatternNever}$`;
