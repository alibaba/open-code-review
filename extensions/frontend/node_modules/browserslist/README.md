# Browserslist

<img width="120" height="120" alt="Browserslist logo by Anton Popov"
     src="https://browsersl.ist/logo.svg" align="right">

The config to share target browsers and Node.js versions between different
front-end tools. It is used in:

- [Autoprefixer]
- [Babel]
- [postcss-preset-env]
- [eslint-plugin-compat]
- [stylelint-no-unsupported-browser-features]
- [postcss-normalize]
- [obsolete-webpack-plugin]

All tools will find target browsers automatically,
when you add the following to `package.json`:

```json
  "browserslist": [
    "defaults and fully supports es6-module",
    "maintained node versions"
  ]
```

Or in `.browserslistrc` config:

```yaml
# Browsers that we support

defaults and fully supports es6-module
maintained node versions
```

Developers set their version lists using queries like `last 2 versions`
to be free from updating versions manually.
Browserslist will use [`caniuse-lite`] with [Can I Use] data for this queries.

You can check how config works at our playground: [`browsersl.ist`](https://browsersl.ist/)

<a href="https://browsersl.ist/">
  <img src="/img/screenshot.webp" alt="browsersl.ist website">
</a>

<br>

---

<img src="https://cdn.evilmartians.com/badges/logo-no-label.svg" alt="" width="22" height="16" />  Browserslist is built by <b><a href="https://evilmartians.com/">Evil Martians</a></b>, an American design and engineering consultancy for <b>developer tools, AI, and cybersecurity startups</b>.

---

[stylelint-no-unsupported-browser-features]: https://github.com/ismay/stylelint-no-unsupported-browser-features
[obsolete-webpack-plugin]: https://github.com/ElemeFE/obsolete-webpack-plugin
[eslint-plugin-compat]: https://github.com/amilajack/eslint-plugin-compat
[postcss-preset-env]: https://github.com/csstools/postcss-plugins/tree/main/plugin-packs/postcss-preset-env
[postcss-normalize]: https://github.com/csstools/postcss-normalize
[`browsersl.ist`]: https://browsersl.ist/
[`caniuse-lite`]: https://github.com/ben-eb/caniuse-lite
[Autoprefixer]: https://github.com/postcss/autoprefixer
[Can I Use]: https://caniuse.com/
[Babel]: https://github.com/babel/babel/tree/master/packages/babel-preset-env

## Docs
Read full docs **[here](https://github.com/browserslist/browserslist#readme)**.
