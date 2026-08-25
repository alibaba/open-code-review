# @istanbuljs/schema

[![NPM Version][npm-image]][npm-url]
[![NPM Downloads][downloads-image]][downloads-url]
[![MIT][license-image]](LICENSE)

Schemas describing various structures used by nyc and istanbuljs

## Usage

```js
const {nyc} = require('@istanbuljs/schema').defaults;

console.log(`Default exclude list:\n\t* ${nyc.exclude.join('\n\t* ')}`);
```

## Testing

- **Runtime support**: See `engines.node` in `package.json` (currently `>=8`).
- **Running tests**: Requires **Node.js >=22.3.0** (uses `node:test` snapshot testing via `t.assert.snapshot`).

```bash
npm test
```

To update snapshots:

```bash
npm run test:update-snapshots
```

## `@istanbuljs/schema` for enterprise

Available as part of the Tidelift Subscription.

The maintainers of `@istanbuljs/schema` and thousands of other packages are working with Tidelift to deliver commercial support and maintenance for the open source dependencies you use to build your applications. Save time, reduce risk, and improve code health, while paying the maintainers of the exact dependencies you use. [Learn more.](https://tidelift.com/subscription/pkg/npm-istanbuljs-schema?utm_source=npm-istanbuljs-schema&utm_medium=referral&utm_campaign=enterprise)

[npm-image]: https://img.shields.io/npm/v/@istanbuljs/schema.svg
[npm-url]: https://npmjs.org/package/@istanbuljs/schema
[downloads-image]: https://img.shields.io/npm/dm/@istanbuljs/schema.svg
[downloads-url]: https://npmjs.org/package/@istanbuljs/schema
[license-image]: https://img.shields.io/npm/l/@istanbuljs/schema.svg
