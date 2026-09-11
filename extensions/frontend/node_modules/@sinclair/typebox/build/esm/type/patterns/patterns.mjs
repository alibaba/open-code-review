export const PatternBoolean = '(true|false)';
export const PatternNumber = '(0|[1-9][0-9]*)';
export const PatternString = '(.*)';
export const PatternNever = '(?!.*)';
export const PatternBooleanExact = `^${PatternBoolean}$`;
export const PatternNumberExact = `^${PatternNumber}$`;
export const PatternStringExact = `^${PatternString}$`;
export const PatternNeverExact = `^${PatternNever}$`;
