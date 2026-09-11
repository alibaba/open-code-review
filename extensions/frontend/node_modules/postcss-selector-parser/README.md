# postcss-selector-parser

[![npm package version](https://img.shields.io/github/package-json/v/postcss/postcss-selector-parser) ![npm downloads](https://img.shields.io/npm/dm/postcss-selector-parser)](https://www.npmjs.com/package/postcss-selector-parser)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/postcss/postcss-selector-parser/test.yml)](https://github.com/postcss/postcss-selector-parser/actions)
[![License](https://img.shields.io/github/license/postcss/postcss-selector-parser)](https://github.com/postcss/postcss-selector-parser)  

> Selector parser with built in methods for working with selector strings.

## Install

With [npm](https://npmjs.com/package/postcss-selector-parser) do:

```
npm install postcss-selector-parser
```

## Quick Start

```js
const parser = require('postcss-selector-parser');
const transform = selectors => {
    selectors.walk(selector => {
        // do something with the selector
        console.log(String(selector))
    });
};

const transformed = parser(transform).processSync('h1, h2, h3');
```

To normalize selector whitespace:

```js
const parser = require('postcss-selector-parser');
const normalized = parser().processSync('h1, h2, h3', {lossless: false});
// -> h1,h2,h3
```

Async support is provided through `parser.process` and will resolve a Promise
with the resulting selector string.

## API

Please see [API.md](API.md).

## Security

### Selector nesting depth (CVE-2026-9358)

The parser walks the selector AST recursively, both when parsing and when
serializing it back to a string (`.toString()`). In versions up to and
including `7.1.1`, a selector with extreme nesting — for example thousands of
nested `:not(...)` — could recurse deeply enough to overflow the call stack and
throw `RangeError: Maximum call stack size exceeded`, a potential
denial-of-service when processing untrusted CSS.

This is now bounded by a maximum nesting depth (default: `256`). Beyond that
depth, parsing and serialization throw a regular, catchable `Error` at a
predictable point instead of relying on the runtime hitting its stack limit.
The default is far above any realistic selector, so it does not affect normal
use.

**Practical impact is low.** The only attacker-controlled input is the selector
string itself, which is now capped by the default limit. The limit is
adjustable through the `maxNestingDepth` option, but that option is trusted
configuration provided by the integrating code — it is never derived from the
parsed CSS, so a malicious selector cannot change it:

```js
// Tighten the limit when parsing untrusted input:
parser().processSync(untrustedSelector, {maxNestingDepth: 128});
```

Raising `maxNestingDepth` to a very large value is an explicit, informed choice
and can reintroduce the stack-overflow risk in environments with a small call
stack (e.g. browser workers). The default is recommended unless you have a
specific need.

## Credits

* Huge thanks to Andrey Sitnik (@ai) for work on PostCSS which helped
  accelerate this module's development.

## License

MIT
