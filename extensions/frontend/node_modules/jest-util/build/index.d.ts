/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {WriteStream} from 'node:tty';
import picomatch from 'picomatch';
import {Config, Global as Global_2} from '@jest/types';

declare const ARROW = ' \u203A ';

/**
 * Whether the given value has properties that can be deleted (regardless of protection).
 *
 * @param value The given value.
 */
export declare function canDeleteProperties(value: unknown): value is object;

declare const CLEAR: string;

export declare function clearLine(stream: WriteStream): void;

export declare function convertDescriptorToString(
  descriptor: Global_2.BlockNameLike | undefined,
): string;

export declare function createDirectory(path: string): void;

export declare function deepCyclicCopy<T>(
  value: T,
  options?: DeepCyclicCopyOptions,
  cycles?: WeakMap<any, any>,
): T;

declare type DeepCyclicCopyOptions = {
  blacklist?: Set<string>;
  keepPrototype?: boolean;
};

/**
 * Deletes all the properties from the given value (if it's an object),
 * unless the value was protected via {@link #protectProperties}.
 *
 * @param value the given value.
 */
export declare function deleteProperties(value: unknown): void;

/**
 *  - <b>off</b>: deletion is completely turned off.
 *  - <b>soft</b>: doesn't delete objects, but instead wraps their getter/setter with a deprecation warning.
 *  - <b>on</b>: actually delete objects (using `delete`).
 */
export declare type DeletionMode = 'soft' | 'off' | 'on';

export declare class ErrorWithStack extends Error {
  constructor(
    message: string | undefined,
    callsite: (...args: Array<any>) => unknown,
    stackLimit?: number,
  );
}

export declare function formatTime(
  time: number,
  prefixPower?: number,
  padLeftLength?: number,
): string;

/**
 * Converts a list of globs into a function that matches a path against the
 * globs.
 *
 * Every time picomatch is called, it will parse the glob strings and turn
 * them into regexp instances. Instead of calling picomatch repeatedly with
 * the same globs, we can use this function which will build the picomatch
 * matchers ahead of time and then have an optimized path for determining
 * whether an individual path matches.
 *
 * This function is intended to match the behavior of `micromatch()`.
 *
 * @example
 * const isMatch = globsToMatcher(['*.js', '!*.test.js']);
 * isMatch('pizza.js'); // true
 * isMatch('pizza.test.js'); // false
 */
export declare function globsToMatcher(
  globs: Array<string>,
  picomatchOptions?: picomatch.PicomatchOptions,
): Matcher;

declare const ICONS: {
  failed: string;
  pending: string;
  success: string;
  todo: string;
};

/**
 * Initializes the garbage collection utils with the given deletion mode.
 *
 * @param globalObject the global object on which to store the deletion mode.
 * @param deletionMode the deletion mode to use.
 */
export declare function initializeGarbageCollectionUtils(
  globalObject: typeof globalThis,
  deletionMode: DeletionMode,
): void;

export declare function installCommonGlobals(
  globalObject: typeof globalThis,
  globals: Config.ConfigGlobals,
  garbageCollectionDeletionMode?: DeletionMode,
): typeof globalThis & Config.ConfigGlobals;

export declare function interopRequireDefault(obj: any): any;

export declare function invariant(
  condition: unknown,
  message?: string,
): asserts condition;

export declare const isError: (error: unknown) => error is Error;

export declare const isInteractive: boolean;

export declare function isNonNullable<T>(value: T): value is NonNullable<T>;

export declare function isPromise<T = unknown>(
  candidate: unknown,
): candidate is PromiseLike<T>;

declare type Matcher = (str: string) => boolean;

export declare function pluralize(
  word: string,
  count: number,
  ending?: string,
): string;

declare namespace preRunMessage {
  export {print_2 as print, remove};
}
export {preRunMessage};

declare function print_2(stream: WriteStream): void;

/**
 * Protects the given value from being deleted by {@link #deleteProperties}.
 *
 * @param value The given value.
 * @param properties If the array contains any property,
 * then only these properties will be protected; otherwise if the array is empty,
 * all properties will be protected.
 * @param depth Determines how "deep" the protection should be.
 * A value of 0 means that only the top-most properties will be protected,
 * while a value larger than 0 means that deeper levels of nesting will be protected as well.
 */
export declare function protectProperties<T>(
  value: T,
  properties?: Array<keyof T>,
  depth?: number,
): boolean;

declare function remove(stream: WriteStream): void;

export declare function replacePathSepForGlob(path: string): string;

export declare function requireOrImportModule<T>(
  filePath: string,
  applyInteropRequireDefault?: boolean,
): Promise<T>;

export declare function setGlobal(
  globalToMutate: typeof globalThis | Global_2.Global,
  key: string | symbol,
  value: unknown,
  afterTeardown?: 'clean' | 'retain',
): void;

declare namespace specialChars {
  export {ARROW, ICONS, CLEAR};
}
export {specialChars};

export declare function tryRealpath(path: string): string;
