export type Task<T> = () => Promise<T>;
export type FunctionReturning<T> = () => T;
export type ExtractCommentsOptions =
  import("./index.js").ExtractCommentsOptions;
export type ExtractCommentsFunction =
  import("./index.js").ExtractCommentsFunction;
export type ExtractCommentsCondition =
  import("./index.js").ExtractCommentsCondition;
export type Input = import("./index.js").Input;
export type MinimizedResult = import("./index.js").MinimizedResult;
export type CustomOptions = import("./index.js").CustomOptions;
export type RawSourceMap = import("./index.js").RawSourceMap;
export type EXPECTED_OBJECT = import("./index.js").EXPECTED_OBJECT;
export type ExtractedComments = string[];
/**
 * Minify CSS using `clean-css`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function cleanCssMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace cleanCssMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker threads are supported
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * Minify CSS using `cssnano` (via `postcss`).
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function cssnanoMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace cssnanoMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker threads are supported
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * Minify CSS using `csso`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function cssoMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace cssoMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker threads are supported
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function esbuildMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace esbuildMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker thread is supported, false otherwise
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a JavaScript file
   */
  function filter(name: string): boolean;
}
/**
 * Minify CSS using `esbuild` (with the CSS loader).
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function esbuildMinifyCss(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace esbuildMinifyCss {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `esbuild` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * Map a webpack `output.environment` configuration to the highest
 * ECMAScript version that the target is known to support. Returns `5`
 * when no ES2015+ features are flagged.
 * @param {NonNullable<NonNullable<import("webpack").Configuration["output"]>["environment"]>} environment environment
 * @returns {number} ecma version (5, 2015, 2017 or 2020)
 */
export function getEcmaVersion(
  environment: NonNullable<
    NonNullable<import("webpack").Configuration["output"]>["environment"]
  >,
): number;
/**
 * Minify HTML using `html-minifier-terser`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map (ignored for HTML)
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function htmlMinifierTerser(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace htmlMinifierTerser {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker threads are supported
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like an HTML file
   */
  function filter(name: string): boolean;
}
/**
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function jsonMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace jsonMinify {
  function getMinimizerVersion(): string;
  function supportsWorker(): boolean;
  function supportsWorkerThreads(): boolean;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a JSON file
   */
  function filter(name: string): boolean;
}
/**
 * Minify CSS using `lightningcss`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function lightningCssMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace lightningCssMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `lightningcss` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * @template T
 * @typedef {() => T} FunctionReturning
 */
/**
 * @template T
 * @param {FunctionReturning<T>} fn memorized function
 * @returns {FunctionReturning<T>} new function
 */
export function memoize<T>(fn: FunctionReturning<T>): FunctionReturning<T>;
/**
 * Minify HTML using `@minify-html/node`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map (ignored for HTML)
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function minifyHtmlNode(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace minifyHtmlNode {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `@minify-html/node` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like an HTML file
   */
  function filter(name: string): boolean;
}
/**
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @param {ExtractCommentsOptions=} extractComments extract comments option
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function swcMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
  extractComments?: ExtractCommentsOptions | undefined,
): Promise<MinimizedResult>;
export namespace swcMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker thread is supported, false otherwise
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a JavaScript file
   */
  function filter(name: string): boolean;
}
/**
 * Minify CSS using `@swc/css`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function swcMinifyCss(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace swcMinifyCss {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `@swc/css` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a CSS file
   */
  function filter(name: string): boolean;
}
/**
 * Minify a complete HTML document using `@swc/html`.
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map (ignored for HTML)
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function swcMinifyHtml(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace swcMinifyHtml {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `@swc/html` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like an HTML file
   */
  function filter(name: string): boolean;
}
/**
 * Minify an HTML fragment using `@swc/html`.
 *
 * Use this for partial HTML (e.g. inside `<template></template>` tags or
 * HTML strings that are inserted into another document).
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map (ignored for HTML)
 * @param {CustomOptions=} minimizerOptions options
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function swcMinifyHtmlFragment(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
): Promise<MinimizedResult>;
export namespace swcMinifyHtmlFragment {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} false because `@swc/html` is a native binding
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like an HTML file
   */
  function filter(name: string): boolean;
}
/**
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @param {ExtractCommentsOptions=} extractComments extract comments option
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function terserMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
  extractComments?: ExtractCommentsOptions | undefined,
): Promise<MinimizedResult>;
export namespace terserMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker thread is supported, false otherwise
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a JavaScript file
   */
  function filter(name: string): boolean;
}
/**
 * @template T
 * @typedef {() => Promise<T>} Task
 */
/**
 * Run tasks with limited concurrency.
 * @template T
 * @param {number} limit Limit of tasks that run at once.
 * @param {Task<T>[]} tasks List of tasks to run.
 * @returns {Promise<T[]>} A promise that fulfills to an array of the results
 */
export function throttleAll<T>(limit: number, tasks: Task<T>[]): Promise<T[]>;
/**
 * @param {Input} input input
 * @param {RawSourceMap=} sourceMap source map
 * @param {CustomOptions=} minimizerOptions options
 * @param {ExtractCommentsOptions=} extractComments extract comments option
 * @returns {Promise<MinimizedResult>} minimized result
 */
export function uglifyJsMinify(
  input: Input,
  sourceMap?: RawSourceMap | undefined,
  minimizerOptions?: CustomOptions | undefined,
  extractComments?: ExtractCommentsOptions | undefined,
): Promise<MinimizedResult>;
export namespace uglifyJsMinify {
  /**
   * @returns {string | undefined} the minimizer version
   */
  function getMinimizerVersion(): string | undefined;
  /**
   * @returns {boolean | undefined} true if worker thread is supported, false otherwise
   */
  function supportsWorkerThreads(): boolean | undefined;
  /**
   * @param {string} name asset name
   * @returns {boolean} true if `name` looks like a JavaScript file
   */
  function filter(name: string): boolean;
}
