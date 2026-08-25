<div align="center">
  <a href="https://github.com/webpack/webpack">
    <img width="200" height="200" src="https://webpack.js.org/assets/icon-square-big.svg">
  </a>
</div>

[![npm][npm]][npm-url]
[![node][node]][node-url]
[![tests][tests]][tests-url]
[![cover][cover]][cover-url]
[![discussion][discussion]][discussion-url]
[![size][size]][size-url]

# minimizer-webpack-plugin

This plugin minifies your assets in a webpack build. It ships with several
built-in minimizers covering JavaScript, JSON, HTML, and CSS — pick one
with the [`minify`](#minify) option and target the right files with
[`test`](#test).

JavaScript minimizers:

- [`terser`](https://github.com/terser/terser) — `MinimizerPlugin.terserMinify` (default). The same JavaScript-based minifier that webpack uses out of the box; produces small, well-tested output and supports the full set of `extractComments` modes.
- [`uglify-js`](https://github.com/mishoo/UglifyJS) — `MinimizerPlugin.uglifyJsMinify`. ES5-only minifier, useful when you specifically need UglifyJS-compatible output. Requires `npm install --save-dev uglify-js`.
- [`@swc/core`](https://github.com/swc-project/swc) — `MinimizerPlugin.swcMinify`. A very fast Rust-based JavaScript/TypeScript minifier. Requires `npm install --save-dev @swc/core`.
- [`esbuild`](https://github.com/evanw/esbuild) — `MinimizerPlugin.esbuildMinify`. An extremely fast JS bundler/minifier; legal comments are always preserved (no `extractComments` support). Requires `npm install --save-dev esbuild`.

JSON minimizer:

- `JSON.stringify` — `MinimizerPlugin.jsonMinify`. Built in (no extra dependency); supports `space` and `replacer` options.

HTML minimizers:

- [`html-minifier-terser`](https://github.com/terser/html-minifier-terser) — `MinimizerPlugin.htmlMinifierTerser`. The default HTML minimizer. JavaScript-based, no native dependency. Requires `npm install --save-dev html-minifier-terser`.
- [`@swc/html`](https://github.com/swc-project/swc) — `MinimizerPlugin.swcMinifyHtml` (full HTML documents) and `MinimizerPlugin.swcMinifyHtmlFragment` (HTML fragments, e.g. `<template>` content). Very fast Rust-based platform for the Web. Requires `npm install --save-dev @swc/html`.
- [`@minify-html/node`](https://github.com/wilsonzlin/minify-html) — `MinimizerPlugin.minifyHtmlNode`. A Rust HTML minifier optimised for speed and effectiveness. Requires `npm install --save-dev @minify-html/node`.

CSS minimizers:

- [`cssnano`](https://cssnano.github.io/cssnano/) — `MinimizerPlugin.cssnanoMinify`. The default CSS minimizer. Built on top of [PostCSS](https://postcss.org/). Requires `npm install --save-dev cssnano postcss`.
- [`csso`](https://github.com/css/csso) — `MinimizerPlugin.cssoMinify`. A CSS minifier with structural optimisations. Requires `npm install --save-dev csso`.
- [`clean-css`](https://github.com/clean-css/clean-css) — `MinimizerPlugin.cleanCssMinify`. A widely-used CSS optimiser. Requires `npm install --save-dev clean-css`.
- [`esbuild`](https://github.com/evanw/esbuild) — `MinimizerPlugin.esbuildMinifyCss`. Very fast CSS minification using esbuild's CSS loader. Requires `npm install --save-dev esbuild`.
- [`lightningcss`](https://github.com/parcel-bundler/lightningcss) — `MinimizerPlugin.lightningCssMinify`. A Rust-based CSS parser, transformer, and minifier. Requires `npm install --save-dev lightningcss`.
- [`@swc/css`](https://github.com/swc-project/swc) — `MinimizerPlugin.swcMinifyCss`. A very fast Rust-based CSS minifier. Requires `npm install --save-dev @swc/css`.

All of the non-default minimizers are declared as **optional** peer
dependencies — install only the ones you actually use. You can also stack
multiple `MinimizerPlugin` instances in the same build to handle different
file types with different minimizers (see [Examples](#examples)).

## Getting Started

Webpack v5 comes with the latest `minimizer-webpack-plugin` out of the box.
If you are using Webpack v5 or above and wish to customize the options, you will still need to install `minimizer-webpack-plugin`.
Using Webpack v4, you have to install `terser-webpack-plugin` v4 (`minimizer-webpack-plugin` is only published for Webpack v5+).

To begin, you'll need to install `minimizer-webpack-plugin`:

```console
npm install minimizer-webpack-plugin --save-dev
```

or

```console
yarn add -D minimizer-webpack-plugin
```

or

```console
pnpm add -D minimizer-webpack-plugin
```

Then add the plugin to your `webpack` configuration. For example:

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [new MinimizerPlugin()],
  },
};
```

Finally, run `webpack` using the method you normally use (e.g., via CLI or an npm script).

## Note about source maps

**Works only with `source-map`, `inline-source-map`, `hidden-source-map` and `nosources-source-map` values for the [`devtool`](https://webpack.js.org/configuration/devtool/) option.**

Why?

- `eval` wraps modules in `eval("string")` and the minimizer does not handle strings.
- `cheap` has no column information and the minimizer generates only a single line, which leaves only a single mapping.

Using supported `devtool` values enable source map generation.

## Options

- **[`test`](#test)**
- **[`include`](#include)**
- **[`exclude`](#exclude)**
- **[`parallel`](#parallel)**
- **[`minify`](#minify)**
- **[`minimizerOptions`](#minimizeroptions)**
- **[`extractComments`](#extractcomments)**

### `test`

Type:

```ts
type test = string | RegExp | (string | RegExp)[];
```

Default: `/\.m?js(\?.*)?$/i`

Test to match files against.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        test: /\.js(\?.*)?$/i,
      }),
    ],
  },
};
```

### `include`

Type:

```ts
type include = string | RegExp | (string | RegExp)[];
```

Default: `undefined`

Files to include.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        include: /\/includes/,
      }),
    ],
  },
};
```

### `exclude`

Type:

```ts
type exclude = string | RegExp | (string | RegExp)[];
```

Default: `undefined`

Files to exclude.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        exclude: /\/excludes/,
      }),
    ],
  },
};
```

### `parallel`

Type:

```ts
type parallel = boolean | number;
```

Default: `true`

Use multi-process parallel running to improve the build speed.

Default number of concurrent runs: `os.cpus().length - 1` or `os.availableParallelism() - 1` (if this function is supported).

> **Note**
>
> Parallelization can speedup your build significantly and is therefore **highly recommended**.

> **Warning**
>
> If you use **Circle CI** or any other environment that doesn't provide the real available count of CPUs then you need to explicitly set up the number of CPUs to avoid `Error: Call retries were exceeded` (see [#143](https://github.com/webpack/minimizer-webpack-plugin/issues/143), [#202](https://github.com/webpack/minimizer-webpack-plugin/issues/202)).

#### `boolean`

Enable/disable multi-process parallel running.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        parallel: true,
      }),
    ],
  },
};
```

#### `number`

Enable multi-process parallel running and set number of concurrent runs.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        parallel: 4,
      }),
    ],
  },
};
```

### `minify`

Type:

```ts
type minifyFn = (
  input: Record<string, string>,
  sourceMap: import("@jridgewell/trace-mapping").SourceMapInput | undefined,
  minifyOptions: {
    module?: boolean | undefined;
    ecma?: import("terser").ECMA | undefined;
  },
  extractComments:
    | boolean
    | "all"
    | "some"
    | RegExp
    | ((
        astNode: any,
        comment: {
          value: string;
          type: "comment1" | "comment2" | "comment3" | "comment4";
          pos: number;
          line: number;
          col: number;
        },
      ) => boolean)
    | {
        condition?:
          | boolean
          | "all"
          | "some"
          | RegExp
          | ((
              astNode: any,
              comment: {
                value: string;
                type: "comment1" | "comment2" | "comment3" | "comment4";
                pos: number;
                line: number;
                col: number;
              },
            ) => boolean)
          | undefined;
        filename?: string | ((fileData: any) => string) | undefined;
        banner?:
          | string
          | boolean
          | ((commentsFile: string) => string)
          | undefined;
      }
    | undefined,
) => Promise<{
  code: string;
  map?: import("@jridgewell/trace-mapping").SourceMapInput | undefined;
  errors?: (string | Error)[] | undefined;
  warnings?: (string | Error)[] | undefined;
  extractedComments?: string[] | undefined;
}>;

type minify = minifyFn | minifyFn[];
```

Default: `MinimizerPlugin.terserMinify`

Allows you to override the default minify function.
By default plugin uses [terser](https://github.com/terser/terser) package.
Useful for using and testing unpublished versions or forks.

An array of functions can also be provided. Each minimizer can expose a
`filter(name, info)` helper that decides whether it should run on a given
asset; the plugin dispatches each asset only to the minimizers whose `filter`
accepts it (or runs them all when no filter is set). All built-in minimizers
ship with a `filter` that matches their natural extension, so a single plugin
instance and a single worker pool can handle JS, CSS, HTML and JSON together
without juggling multiple `MinimizerPlugin` instances — just widen `test` to
let those asset types reach the dispatcher:

```js
new MinimizerPlugin({
  test: /\.(?:[cm]?js|css|html?|json)(\?.*)?$/i,
  minify: [
    MinimizerPlugin.terserMinify,
    MinimizerPlugin.cssnanoMinify,
    MinimizerPlugin.htmlMinifierTerser,
    MinimizerPlugin.jsonMinify,
  ],
});
```

When more than one minimizer in the array claims the same asset, the chain
semantic still applies: the output of each accepting minimizer is fed as
input to the next. The [`minimizerOptions`](#minimizeroptions) option may
be an array (index-paired with `minify`) or a single object reused by every
minimizer.

The `test` option always defaults to `/\.[cm]?js(\?.*)?$/i`. When you mix
asset types in a single plugin instance, widen `test` so non-JS assets reach
the dispatcher (for example `test: /\.(?:[cm]?js|css|html?|json)(\?.*)?$/i`).

> **Warning**
>
> **Always use `require` inside `minify` function when `parallel` option enabled**.

#### `function`

**webpack.config.js**

```js
// Can be async
const minify = (input, sourceMap, minimizerOptions, extractsComments) => {
  // The `minimizerOptions` argument contains options from the `minimizerOptions` plugin option
  // You can use `minimizerOptions.myCustomOption`

  // Custom logic for extract comments
  const { map, code } = require("uglify-module") // Or require('./path/to/uglify-module')
    .minify(input, {
      /* Your options for minification */
    });

  return { map, code, warnings: [], errors: [], extractedComments: [] };
};

// Used to regenerate `fullhash`/`chunkhash` between different implementation
// Example: you fix a bug in custom minimizer/custom function, but unfortunately webpack doesn't know about it, so you will get the same fullhash/chunkhash
// to avoid this you can provide version of your custom minimizer
// You don't need if you use only `contenthash`
minify.getMinimizerVersion = () => {
  let packageJson;

  try {
    packageJson = require("uglify-module/package.json");
  } catch (error) {
    // Ignore
  }

  return packageJson && packageJson.version;
};

// Restrict the minimizer to the assets it can actually handle. The plugin
// skips assets for which `filter` returns `false` and (when an array of
// minimizers is used) dispatches each asset only to the minimizers that
// accept it. Returning `undefined` is treated as accept.
minify.filter = (name) => /\.[cm]?js(\?.*)?$/i.test(name);

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minimizerOptions: {
          myCustomOption: true,
        },
        minify,
      }),
    ],
  },
};
```

#### `array`

If an array of functions is passed to the `minify` option, each asset is
dispatched to the minimizers whose `filter` accepts it. When more than one
minimizer accepts the same asset the output of each is fed as input to the
next one (the chain semantic). The `minimizerOptions` option can be either an
array of option objects (index-paired with `minify`) or a single object that
will be shared by all minimizers. Warnings, errors and extracted comments
from all running minimizers are merged together.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minify: [MinimizerPlugin.terserMinify, MinimizerPlugin.swcMinify],
        // `minimizerOptions` can be an array of options, one per `minify` entry
        minimizerOptions: [
          // Options for `MinimizerPlugin.terserMinify`
          { mangle: false },
          // Options for `MinimizerPlugin.swcMinify`
          {},
        ],
      }),
    ],
  },
};
```

A single plugin instance can also handle multiple asset types — the built-in
minimizers each ship with a `filter` matching their natural extension, so JS,
CSS, HTML and JSON can all be minified by one shared worker pool:

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        // `test` still defaults to JS only, so widen it to catch every
        // asset type you want the dispatcher to consider.
        test: /\.(?:[cm]?js|css|html?|json)(\?.*)?$/i,
        minify: [
          MinimizerPlugin.terserMinify,
          MinimizerPlugin.cssnanoMinify,
          MinimizerPlugin.htmlMinifierTerser,
          MinimizerPlugin.jsonMinify,
        ],
      }),
    ],
  },
};
```

### `minimizerOptions`

Type:

```ts
interface minimizerOptions {
  compress?: boolean | CompressOptions;
  ecma?: ECMA;
  enclose?: boolean | string;
  ie8?: boolean;
  keep_classnames?: boolean | RegExp;
  keep_fnames?: boolean | RegExp;
  mangle?: boolean | MangleOptions;
  module?: boolean;
  nameCache?: object;
  format?: FormatOptions;
  /** @deprecated */
  output?: FormatOptions;
  parse?: ParseOptions;
  safari10?: boolean;
  sourceMap?: boolean | SourceMapOptions;
  toplevel?: boolean;
}

type options = minimizerOptions | minimizerOptions[];
```

Default: [default](https://github.com/terser/terser#minify-options)

Options for the active minimizer. With the default Terser minify, see Terser's
[minify options](https://github.com/terser/terser#minify-options).

When the [`minify`](#minify) option is an array of minimizers, `minimizerOptions`
can also be an array. Each element is passed to the minimizer at the same
index in the `minify` array. If a single object is provided instead, it is
reused for every minimizer.

> **Note**
>
> `terserOptions` is kept as a deprecated alias of `minimizerOptions` for
> backwards compatibility — passing either is equivalent. If both are set,
> `minimizerOptions` wins. Prefer `minimizerOptions` in new code.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minimizerOptions: {
          ecma: undefined,
          parse: {},
          compress: {},
          mangle: true, // Note `mangle.properties` is `false` by default.
          module: false,
          // Deprecated
          output: null,
          format: null,
          toplevel: false,
          nameCache: null,
          ie8: false,
          keep_classnames: undefined,
          keep_fnames: false,
          safari10: false,
        },
      }),
    ],
  },
};
```

### `extractComments`

Type:

```ts
type extractComments =
  | boolean
  | string
  | RegExp
  | ((
      astNode: any,
      comment: {
        value: string;
        type: "comment1" | "comment2" | "comment3" | "comment4";
        pos: number;
        line: number;
        col: number;
      },
    ) => boolean)
  | {
      condition?:
        | boolean
        | "all"
        | "some"
        | RegExp
        | ((
            astNode: any,
            comment: {
              value: string;
              type: "comment1" | "comment2" | "comment3" | "comment4";
              pos: number;
              line: number;
              col: number;
            },
          ) => boolean)
        | undefined;
      filename?: string | ((fileData: any) => string) | undefined;
      banner?:
        | string
        | boolean
        | ((commentsFile: string) => string)
        | undefined;
    };
```

Default: `true`

Whether comments shall be extracted to a separate file, (see [details](https://github.com/webpack/webpack/commit/71933e979e51c533b432658d5e37917f9e71595a)).

By default, extract only comments using `/^\**!|@preserve|@license|@cc_on/i` RegExp condition and remove remaining comments.

If the original file is named `foo.js`, then the comments will be stored to `foo.js.LICENSE.txt`.

The `minimizerOptions.format.comments` option specifies whether the comment will be preserved - i.e., it is possible to preserve some comments (e.g. annotations) while extracting others, or even preserve comments that have already been extracted.

#### `boolean`

Enable/disable extracting comments.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: true,
      }),
    ],
  },
};
```

#### `string`

Extract `all` or `some` (use the `/^\**!|@preserve|@license|@cc_on/i` RegExp) comments.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: "all",
      }),
    ],
  },
};
```

#### `RegExp`

All comments that match the given expression will be extracted to a separate file.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: /@extract/i,
      }),
    ],
  },
};
```

#### `function`

All comments that match the given expression will be extracted to a separate file.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: (astNode, comment) => {
          if (/@extract/i.test(comment.value)) {
            return true;
          }

          return false;
        },
      }),
    ],
  },
};
```

#### `object`

Allows you to customize condition for extracting comments, and specify the extracted file name and banner.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: {
          condition: /^\**!|@preserve|@license|@cc_on/i,
          filename: (fileData) =>
            // The "fileData" argument contains object with "filename", "basename", "query" and "hash"
            `${fileData.filename}.LICENSE.txt${fileData.query}`,
          banner: (licenseFile) =>
            `License information can be found in ${licenseFile}`,
        },
      }),
    ],
  },
};
```

##### `condition`

Type:

```ts
type condition =
  | boolean
  | "all"
  | "some"
  | RegExp
  | ((
      astNode: any,
      comment: {
        value: string;
        type: "comment1" | "comment2" | "comment3" | "comment4";
        pos: number;
        line: number;
        col: number;
      },
    ) => boolean)
  | undefined;
```

The condition that determines which comments should be extracted.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: {
          condition: "some",
          filename: (fileData) =>
            // The "fileData" argument contains object with "filename", "basename", "query" and "hash"
            `${fileData.filename}.LICENSE.txt${fileData.query}`,
          banner: (licenseFile) =>
            `License information can be found in ${licenseFile}`,
        },
      }),
    ],
  },
};
```

##### `filename`

Type:

```ts
type filename = string | ((fileData: any) => string) | undefined;
```

Default: `[file].LICENSE.txt[query]`

Available placeholders: `[file]`, `[query]` and `[filebase]` (`[base]` for webpack 5).

The file where the extracted comments will be stored.

Default is to append the suffix `.LICENSE.txt` to the original filename.

> **Warning**
>
> We highly recommend using the `.txt` extension. Using `.js`/`.cjs`/`.mjs` extensions may conflict with existing assets, which leads to broken code.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: {
          condition: /^\**!|@preserve|@license|@cc_on/i,
          filename: "extracted-comments.js",
          banner: (licenseFile) =>
            `License information can be found in ${licenseFile}`,
        },
      }),
    ],
  },
};
```

##### `banner`

Type:

```ts
type banner = string | boolean | ((commentsFile: string) => string) | undefined;
```

Default: `/*! For license information please see ${commentsFile} */`

The banner text that points to the extracted file and will be added at the top of the original file.

It can be `false` (no banner), a `String`, or a `function<(string) -> String>` that will be called with the filename where the extracted comments have been stored.

The banner will be wrapped in a comment.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        extractComments: {
          condition: true,
          filename: (fileData) =>
            // The "fileData" argument contains object with "filename", "basename", "query" and "hash"
            `${fileData.filename}.LICENSE.txt${fileData.query}`,
          banner: (commentsFile) =>
            `My custom banner about license information ${commentsFile}`,
        },
      }),
    ],
  },
};
```

## Examples

### Preserve Comments

Extract all legal comments (i.e. `/^\**!|@preserve|@license|@cc_on/i`) and preserve `/@license/i` comments.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minimizerOptions: {
          format: {
            comments: /@license/i,
          },
        },
        extractComments: true,
      }),
    ],
  },
};
```

### Remove Comments

If you want to build without comments, use this config:

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minimizerOptions: {
          format: {
            comments: false,
          },
        },
        extractComments: false,
      }),
    ],
  },
};
```

### [`uglify-js`](https://github.com/mishoo/UglifyJS)

[`UglifyJS`](https://github.com/mishoo/UglifyJS) is a JavaScript parser, minifier, compressor and beautifier toolkit.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minify: MinimizerPlugin.uglifyJsMinify,
        // `minimizerOptions` will be passed to `uglify-js`
        // Link to options - https://github.com/mishoo/UglifyJS#minify-options
        minimizerOptions: {},
      }),
    ],
  },
};
```

### [`swc`](https://github.com/swc-project/swc)

[`swc`](https://github.com/swc-project/swc) is a super-fast compiler written in `Rust`, producing widely supported JavaScript from modern standards and TypeScript.

> **Warning**
>
> `extractComments` is supported with `@swc/core >= 1.15.30`.
> Only serializable extract conditions are supported: booleans, `"some"`, `"all"`, string patterns, `RegExp` values without flags, or object conditions that resolve to those forms.
> Function conditions and flagged regular expressions are not supported.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minify: MinimizerPlugin.swcMinify,
        // `minimizerOptions` will be passed to `swc` (`@swc/core`)
        // Link to options - https://swc.rs/docs/config-js-minify
        minimizerOptions: {},
      }),
    ],
  },
};
```

### [`esbuild`](https://github.com/evanw/esbuild)

[`esbuild`](https://github.com/evanw/esbuild) is an extremely fast JavaScript bundler and minifier.

> **Warning**
>
> The `extractComments` option is not supported, and all legal comments (i.e. copyright, licenses and etc) will be preserved.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minify: MinimizerPlugin.esbuildMinify,
        // `minimizerOptions` will be passed to `esbuild`
        // Link to options - https://esbuild.github.io/api/#minify
        // Note: the `minify` options is true by default (and override other `minify*` options), so if you want to disable the `minifyIdentifiers` option (or other `minify*` options) please use:
        // minimizerOptions: {
        //   minify: false,
        //   minifyWhitespace: true,
        //   minifyIdentifiers: false,
        //   minifySyntax: true,
        // },
        minimizerOptions: {},
      }),
    ],
  },
};
```

### JSON

Uses `JSON.stringify()` to minify your JSON files during the build process.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      // Keeps original terser plugin to minify JS files
      "...",
      // Will minify JSON files (they can come from copy-webpack-plugin or when you are using asset modules)
      new MinimizerPlugin({
        test: /\.json$/,
        minify: MinimizerPlugin.jsonMinify,
        // We are supporting `space` and `replacer` options, you can set them below
        minimizerOptions: {},
      }),
    ],
  },
};
```

### HTML

The plugin can minify HTML assets too. Pick one of the bundled HTML
minimizers and set `test` to match your HTML files.

Available HTML minimizers:

- `MinimizerPlugin.htmlMinifierTerser` — uses [`html-minifier-terser`](https://github.com/terser/html-minifier-terser).
- `MinimizerPlugin.swcMinifyHtml` — uses [`@swc/html`](https://github.com/swc-project/swc) for full HTML documents (with doctype and `<html>`/`<head>`/`<body>` tags).
- `MinimizerPlugin.swcMinifyHtmlFragment` — uses [`@swc/html`](https://github.com/swc-project/swc) for HTML fragments (e.g. content inside `<template></template>` or partial HTML strings).
- `MinimizerPlugin.minifyHtmlNode` — uses [`@minify-html/node`](https://github.com/wilsonzlin/minify-html).

The HTML minimizers are optional peer dependencies — install only the one
you actually use:

```console
npm install --save-dev html-minifier-terser
# or
npm install --save-dev @swc/html
# or
npm install --save-dev @minify-html/node
```

> **Note**
>
> HTML assets typically come from plugins like
> [`copy-webpack-plugin`](https://github.com/webpack-contrib/copy-webpack-plugin),
> [`html-webpack-plugin`](https://github.com/jantimon/html-webpack-plugin),
> or webpack's [asset modules](https://webpack.js.org/guides/asset-modules/).

> **Note**
>
> Whitespace handling differs between tools (defaults):
>
> - `@swc/html` — removes/collapses whitespace only in safe places (around `html`/`body`, inside `<head>`, between `<meta>`/`<script>`/`<link>` etc.).
> - `html-minifier-terser` — always collapses multiple whitespaces to a single space (never removes entirely); configurable via [its options](https://github.com/terser/html-minifier-terser#options-quick-reference).
> - `@minify-html/node` — see [its whitespace docs](https://github.com/wilsonzlin/minify-html#whitespace).

#### `html-minifier-terser`

[`html-minifier-terser`](https://github.com/terser/html-minifier-terser) is a JavaScript-based HTML minifier with no native dependency. It's the default HTML minimizer.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      // Keeps the default Terser plugin for JS files
      "...",
      new MinimizerPlugin({
        test: /\.html(\?.*)?$/i,
        minify: MinimizerPlugin.htmlMinifierTerser,
        // Options - https://github.com/terser/html-minifier-terser#options-quick-reference
        minimizerOptions: {
          collapseWhitespace: true,
          removeComments: true,
        },
      }),
    ],
  },
};
```

#### `@swc/html` — HTML documents

Use `swcMinifyHtml` for complete HTML documents (i.e. with a doctype and `<html>`/`<head>`/`<body>` tags).

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.html(\?.*)?$/i,
        minify: MinimizerPlugin.swcMinifyHtml,
        // Options - https://github.com/swc-project/bindings/blob/main/packages/html/index.ts
        minimizerOptions: {},
      }),
    ],
  },
};
```

#### `@swc/html` — HTML fragments

Use `swcMinifyHtmlFragment` for partial HTML — for example, content of `<template></template>` tags or HTML strings that get injected into another document.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.template\.html$/i,
        minify: MinimizerPlugin.swcMinifyHtmlFragment,
        // Options - https://github.com/swc-project/bindings/blob/main/packages/html/index.ts
        minimizerOptions: {},
      }),
    ],
  },
};
```

> **Note**
>
> The difference between `swcMinifyHtml` and `swcMinifyHtmlFragment` is the
> error reporting — invalid or broken syntax is reported at build time.

#### `@minify-html/node`

[`@minify-html/node`](https://github.com/wilsonzlin/minify-html) is a Rust HTML minifier.

**webpack.config.js**

```js
const Minimizer = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new Minimizer({
        test: /\.html(\?.*)?$/i,
        minify: Minimizer.minifyHtmlNode,
        // Options - https://github.com/wilsonzlin/minify-html#minification
        minimizerOptions: {},
      }),
    ],
  },
};
```

You can also stack multiple `MinimizerPlugin` instances to compress different files with different `minify` functions in the same build (e.g. JS with `terserMinify`, HTML with `htmlMinifierTerser`, JSON with `jsonMinify`).

### CSS

The plugin can minify CSS assets too. Pick one of the bundled CSS
minimizers and set `test` to match your CSS files.

Available CSS minimizers:

- `MinimizerPlugin.cssnanoMinify` — uses [`cssnano`](https://cssnano.github.io/cssnano/) (via [`postcss`](https://postcss.org/)).
- `MinimizerPlugin.cssoMinify` — uses [`csso`](https://github.com/css/csso).
- `MinimizerPlugin.cleanCssMinify` — uses [`clean-css`](https://github.com/clean-css/clean-css).
- `MinimizerPlugin.esbuildMinifyCss` — uses [`esbuild`](https://github.com/evanw/esbuild) with the CSS loader.
- `MinimizerPlugin.lightningCssMinify` — uses [`lightningcss`](https://github.com/parcel-bundler/lightningcss).
- `MinimizerPlugin.swcMinifyCss` — uses [`@swc/css`](https://github.com/swc-project/swc).

The CSS minimizers are optional peer dependencies — install only the ones
you actually use:

```console
npm install --save-dev cssnano postcss
# or
npm install --save-dev csso
# or
npm install --save-dev clean-css
# or
npm install --save-dev esbuild
# or
npm install --save-dev lightningcss
# or
npm install --save-dev @swc/css
```

> **Note**
>
> CSS assets typically come from plugins like
> [`mini-css-extract-plugin`](https://github.com/webpack-contrib/mini-css-extract-plugin)
> or webpack's [asset modules](https://webpack.js.org/guides/asset-modules/).

#### `cssnano`

[`cssnano`](https://cssnano.github.io/cssnano/) is the default CSS minimizer. It runs as a [PostCSS](https://postcss.org/) plugin.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      // Keeps the default Terser plugin for JS files
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cssnanoMinify,
        // Options - https://cssnano.github.io/cssnano/docs/config-file/
        minimizerOptions: {
          preset: "default",
        },
      }),
    ],
  },
};
```

#### `csso`

[`csso`](https://github.com/css/csso) is a CSS minifier with structural optimisations.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cssoMinify,
        // Options - https://github.com/css/csso#minifysource-options
        minimizerOptions: {},
      }),
    ],
  },
};
```

#### `clean-css`

[`clean-css`](https://github.com/clean-css/clean-css) is a widely-used CSS optimiser.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cleanCssMinify,
        // Options - https://github.com/clean-css/clean-css#constructor-options
        minimizerOptions: {},
      }),
    ],
  },
};
```

#### `esbuild`

[`esbuild`](https://github.com/evanw/esbuild) ships with a fast CSS minifier (used via its CSS loader).

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.esbuildMinifyCss,
        // Options - https://esbuild.github.io/api/#transform-api
        minimizerOptions: {},
      }),
    ],
  },
};
```

#### `lightningcss`

[`lightningcss`](https://github.com/parcel-bundler/lightningcss) is a Rust-based CSS parser, transformer, and minifier.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.lightningCssMinify,
        // Options - https://lightningcss.dev/transpilation.html
        minimizerOptions: {},
      }),
    ],
  },
};
```

#### `@swc/css`

[`@swc/css`](https://github.com/swc-project/swc) is a Rust-based CSS minifier.

**webpack.config.js**

```js
const MinimizerPlugin = require("minimizer-webpack-plugin");

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      "...",
      new MinimizerPlugin({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.swcMinifyCss,
        // Options - https://github.com/swc-project/bindings/blob/main/packages/css/index.ts
        minimizerOptions: {},
      }),
    ],
  },
};
```

### Custom Minify Function

Override the default minify function - use `uglify-js` for minification.

**webpack.config.js**

```js
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minify: (file, sourceMap) => {
          // https://github.com/mishoo/UglifyJS2#minify-options
          const uglifyJsOptions = {
            /* your `uglify-js` package options */
          };

          if (sourceMap) {
            uglifyJsOptions.sourceMap = {
              content: sourceMap,
            };
          }

          return require("uglify-js").minify(file, uglifyJsOptions);
        },
      }),
    ],
  },
};
```

### Typescript

With default Terser minify function:

```ts
module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin({
        minimizerOptions: {
          compress: true,
        },
      }),
    ],
  },
};
```

With built-in minify functions:

```ts
import { type JsMinifyOptions as SwcOptions } from "@swc/core";
import { type MinifyOptions as SwcCssOptions } from "@swc/css";
import {
  type FragmentOptions as SwcHtmlFragmentOptions,
  type Options as SwcHtmlOptions,
} from "@swc/html";
import { type OptionsOutput as CleanCssOptions } from "clean-css";
import { type Options as CssnanoOptions } from "cssnano";
import { type CompressOptions as CssoOptions } from "csso";
import { type TransformOptions as EsbuildOptions } from "esbuild";
import { type Options as HtmlMinifierTerserOptions } from "html-minifier-terser";
import { type TransformOptions as LightningCssOptions } from "lightningcss";
import { type MinifyOptions as TerserOptions } from "terser";
import { type MinifyOptions as UglifyJSOptions } from "uglify-js";

module.exports = {
  optimization: {
    minimize: true,
    minimizer: [
      new MinimizerPlugin<SwcOptions>({
        minify: MinimizerPlugin.swcMinify,
        minimizerOptions: {
          // `swc` options
        },
      }),
      new MinimizerPlugin<UglifyJSOptions>({
        minify: MinimizerPlugin.uglifyJsMinify,
        minimizerOptions: {
          // `uglif-js` options
        },
      }),
      new MinimizerPlugin<EsbuildOptions>({
        minify: MinimizerPlugin.esbuildMinify,
        minimizerOptions: {
          // `esbuild` options
        },
      }),

      // Alternative usage:
      new MinimizerPlugin<TerserOptions>({
        minify: MinimizerPlugin.terserMinify,
        minimizerOptions: {
          // `terser` options
        },
      }),

      // HTML minimizers
      new MinimizerPlugin<HtmlMinifierTerserOptions>({
        test: /\.html(\?.*)?$/i,
        minify: MinimizerPlugin.htmlMinifierTerser,
        minimizerOptions: {
          // `html-minifier-terser` options
        },
      }),
      new MinimizerPlugin<SwcHtmlOptions>({
        test: /\.html(\?.*)?$/i,
        minify: MinimizerPlugin.swcMinifyHtml,
        minimizerOptions: {
          // `@swc/html` options
        },
      }),
      new MinimizerPlugin<SwcHtmlFragmentOptions>({
        test: /\.template\.html$/i,
        minify: MinimizerPlugin.swcMinifyHtmlFragment,
        minimizerOptions: {
          // `@swc/html` fragment options
        },
      }),

      // CSS minimizers
      new MinimizerPlugin<CssnanoOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cssnanoMinify,
        minimizerOptions: {
          // `cssnano` options
        },
      }),
      new MinimizerPlugin<CssoOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cssoMinify,
        minimizerOptions: {
          // `csso` options
        },
      }),
      new MinimizerPlugin<CleanCssOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.cleanCssMinify,
        minimizerOptions: {
          // `clean-css` options
        },
      }),
      new MinimizerPlugin<EsbuildOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.esbuildMinifyCss,
        minimizerOptions: {
          // `esbuild` options (CSS loader)
        },
      }),
      new MinimizerPlugin<LightningCssOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.lightningCssMinify,
        minimizerOptions: {
          // `lightningcss` options
        },
      }),
      new MinimizerPlugin<SwcCssOptions>({
        test: /\.css(\?.*)?$/i,
        minify: MinimizerPlugin.swcMinifyCss,
        minimizerOptions: {
          // `@swc/css` options
        },
      }),
    ],
  },
};
```

## Contributing

We welcome all contributions!
If you're new here, please take a moment to review our contributing guidelines before submitting issues or pull requests.

[CONTRIBUTING](https://github.com/webpack/minimizer-webpack-plugin?tab=contributing-ov-file#contributing)

## License

[MIT](./LICENSE)

[npm]: https://img.shields.io/npm/v/minimizer-webpack-plugin.svg
[npm-url]: https://npmjs.com/package/minimizer-webpack-plugin
[node]: https://img.shields.io/node/v/minimizer-webpack-plugin.svg
[node-url]: https://nodejs.org
[tests]: https://github.com/webpack/minimizer-webpack-plugin/workflows/minimizer-webpack-plugin/badge.svg
[tests-url]: https://github.com/webpack/minimizer-webpack-plugin/actions
[cover]: https://codecov.io/gh/webpack/minimizer-webpack-plugin/branch/main/graph/badge.svg
[cover-url]: https://codecov.io/gh/webpack/minimizer-webpack-plugin
[discussion]: https://img.shields.io/github/discussions/webpack/webpack
[discussion-url]: https://github.com/webpack/webpack/discussions
[size]: https://packagephobia.now.sh/badge?p=minimizer-webpack-plugin
[size-url]: https://packagephobia.now.sh/result?p=minimizer-webpack-plugin
