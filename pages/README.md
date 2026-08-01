# OpenCodeReview Landing Page (`pages/`)

This directory contains the OpenCodeReview landing page, built with TypeScript, React, Webpack, and Tailwind CSS.

## Getting Started

### Prerequisites

- Node.js `>=18` (recommended: latest LTS)
- `npm` (comes with Node.js) or `pnpm`

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

Default dev server settings (from `webpack.config.js`):

- URL: `http://localhost:3030`
- Host: `0.0.0.0`
- Port: `3030`

### Build for production

```bash
npx webpack
```

Project script (recommended, sets production mode):

```bash
npm run build
```

Build output is generated in `pages/dist/`.

### Type checking

```bash
npm run typecheck
```

## Project Structure

```text
pages/
├── src/                # React + TypeScript source code
│   ├── components/     # Reusable UI components
│   ├── pages/          # Route-level page components
│   ├── i18n/           # Localization resources and i18n context
│   ├── styles/         # Global styles (Tailwind entry, custom CSS)
│   └── index.tsx       # Frontend entry point
├── dist/               # Production build artifacts (generated)
├── index.html          # HTML template used by HtmlWebpackPlugin
├── webpack.config.js   # Bundling + dev server config
├── tailwind.config.js  # Tailwind theme/content configuration
├── postcss.config.js   # PostCSS pipeline (Tailwind + Autoprefixer)
├── tsconfig.json       # TypeScript compiler options
└── package.json        # Dependencies and scripts
```

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
- Prefer utility-first Tailwind classes and reuse existing design tokens from `tailwind.config.js`.
- Keep imports and file naming consistent with surrounding code.
- Run `npm run typecheck` before opening a PR.

### Tailwind CSS configuration notes

- Tailwind config is in `tailwind.config.js`.
- Content scanning targets:
  - `./src/**/*.{ts,tsx}`
  - `./index.html`
- PostCSS integration is configured in `postcss.config.js` with:
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
- [ ] `npm run typecheck` passes
- [ ] `npm run build` succeeds
- [ ] Before/after screenshots added to PR
- [ ] Scope is limited to one logical change
