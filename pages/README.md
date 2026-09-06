# OpenCodeReview Landing Page (`pages/`)

This directory contains the OpenCodeReview landing page, built with TypeScript, React, Webpack, and Tailwind CSS.

## Getting Started

### Prerequisites

- Node.js `^22.22.2 || ^24.15.0 || >=26` (recommended: latest LTS; every Pages CI job builds on Node 24)
- `npm` (comes with Node.js) or `pnpm`

The floor comes from the dev toolchain rather than the bundler: `jsdom@30`,
the Vitest environment, declares `^22.22.2 || ^24.15.0 || >=26`, and
`eslint@10` declares `^20.19.0 || ^22.13.0 || >=24`. On an older Node,
`npm install` reports `EBADENGINE` and the lint and test toolchain is
outside its supported range.

### Install dependencies

From the `pages/` directory:

```bash
npm install
```

Or with pnpm:

```bash
pnpm install
```

### Start local dev server

```bash
npx webpack serve
```

Equivalent npm script:

```bash
npm run dev
```

Default dev server settings (from `webpack.config.cjs`):

- URL: `http://localhost:3030`
- Host: `0.0.0.0`
- Port: `3030`

### Build for production

On systems where npm scripts run through a POSIX shell (for example Linux and
macOS), use the project script:

```bash
npm run build
```

Build output is generated in `pages/dist/`.

Use the project script or webpack-cli's `--node-env` option rather than calling
Webpack without setting the Node environment. `webpack.config.cjs` derives
`isProduction` from `NODE_ENV` alone, and that one flag decides both Webpack
`mode` and whether `@babel/preset-react` runs its development transform:

| Command                             | Result                                                                                                                                                          |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `npx webpack`                       | Development mode, including the development React build. Do not use this for a production artifact.                                                             |
| `npx webpack --mode production`     | Webpack optimizes the bundle and selects production React, but `NODE_ENV` remains unset for the config, so Babel still enables its React development transform. |
| `npx webpack --node-env production` | Production mode with `NODE_ENV=production`; works across `cmd.exe`, PowerShell, Git Bash, and POSIX shells.                                                     |
| `npm run build`                     | Production mode with `NODE_ENV=production` and `--mode production`.                                                                                             |

For a shell-independent production build, use:

```bash
npx webpack --node-env production
```

webpack-cli sets `process.env.NODE_ENV` before loading the config, so this uses
the intended production configuration without relying on shell-specific inline
environment-variable syntax. The project script itself could be made
cross-platform by using `webpack --node-env production`; changing
`package.json` is intentionally outside the scope of this documentation-only
change.

### Checks

Pages CI runs the following sequence for every PR that touches `pages/**`:

1. `npm run lint`
2. `npm test`
3. `npm run typecheck`
4. `npm run build`
5. Smoke-test the built `dist/`
6. `npm run size`

For local pre-PR validation, run the npm-script checks in the same order. If the
npm script shell does not support POSIX inline environment assignments, use
`npx webpack --node-env production` for the production-build step.

`npm run lint` runs ESLint over `src/`; `npm test` runs the Vitest suite once
(`vitest run`) in a `jsdom` environment; and `npm run size` measures the
combined brotli-compressed size of the files matched by `dist/*.bundle.js`
against the 150 kB budget declared in `package.json`.

## Project Structure

```text
pages/
├── src/                 # React + TypeScript source code
│   ├── components/      # Reusable UI components
│   ├── pages/           # Route-level page components
│   ├── content/         # Bundled Markdown: docs/ and blog/, per locale
│   ├── hooks/           # Custom React hooks
│   ├── i18n/            # Localization resources and i18n context
│   ├── styles/          # Global styles (Tailwind entry, custom CSS)
│   ├── utils/           # Shared helpers
│   ├── assets/          # Imported icons and images
│   └── index.tsx        # Frontend entry point
├── public/              # Static files copied verbatim into dist/
├── dist/                # Production build artifacts (generated, gitignored)
├── index.html           # HTML template used by HtmlWebpackPlugin
├── webpack.config.cjs   # Bundling + dev server config
├── tailwind.config.cjs  # Tailwind theme/content configuration
├── postcss.config.cjs   # PostCSS pipeline (Tailwind + Autoprefixer)
├── eslint.config.js     # ESLint flat config
├── vitest.config.ts     # Vitest config (jsdom environment)
├── vitest.setup.ts      # Test setup (@testing-library/jest-dom)
├── tsconfig.json        # TypeScript compiler options
├── go.mod               # Module boundary only -- no Go code lives here
└── package.json         # Dependencies and scripts
```

Note the mixed extensions. `package.json` sets `"type": "module"`, so a bare
`.js` file is parsed as ESM. `eslint.config.js` is ESM and stays `.js`; the
Webpack, Tailwind and PostCSS configs use `module.exports` and therefore have
to be `.cjs`. `postcss.config.cjs` likewise refers to `./tailwind.config.cjs`
by name.

The Markdown under `src/content/` is imported by `src/content/docs/index.ts`
and `src/content/blog/index.ts` and ends up in the bundle, so it counts
against the size budget. This file does not -- it is a developer guide only.

## Development Guidelines

### PR screenshots are required

Any PR that changes files in `pages/` must include **before/after screenshots** of affected views in the PR description.

Please include:

- What page/section changed
- Before screenshot
- After screenshot
- Desktop or mobile context (if responsive behavior changed)

### Code style and formatting

- Follow existing TypeScript + React style in this directory.
- Keep components focused and readable; prefer splitting large JSX blocks into smaller components.
- Prefer utility-first Tailwind classes and reuse existing design tokens from `tailwind.config.cjs`.
- Keep imports and file naming consistent with surrounding code.
- Run the checks listed above before opening a PR.

### Tailwind CSS configuration notes

- Tailwind config is in `tailwind.config.cjs`.
- Content scanning targets:
  - `./src/**/*.{ts,tsx}`
  - `./index.html`
- PostCSS integration is configured in `postcss.config.cjs` with:
  - `tailwindcss`
  - `autoprefixer`

### TypeScript configuration notes

- TypeScript config is in `tsconfig.json`.
- Important defaults:
  - `strict: true`
  - `jsx: react-jsx`
  - `target: ES2020`
  - `noEmit: true` (Webpack handles output)

## Suggested PR Checklist

- [ ] Dependencies installed and project runs locally
- [ ] `npm run lint` passes
- [ ] `npm test` passes
- [ ] `npm run typecheck` passes
- [ ] Production build succeeds (`npm run build` on a POSIX script shell or `npx webpack --node-env production` as the portable equivalent)
- [ ] `npm run size` passes
- [ ] Before/after screenshots added to PR when an affected view exists
- [ ] Scope is limited to one logical change
