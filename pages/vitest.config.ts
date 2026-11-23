// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';
import { normalizePath } from 'vite';
import react from '@vitejs/plugin-react';
import { englishDocFiles, readEnglishDocIdentifiers } from './english-doc-identifiers.cjs';

export default defineConfig({
  define: { __ENGLISH_DOC_IDENTIFIERS__: JSON.stringify(readEnglishDocIdentifiers()) },
  plugins: [
    react(),
    {
      // Markdown bodies are raw strings under webpack's asset/source rule;
      // mirror that in tests so content modules import the real files.
      name: 'markdown-raw-source',
      configResolved(config) {
        const englishContent = fileURLToPath(new URL('./src/content/docs/en.ts', import.meta.url));
        config.configFileDependencies.push(...[englishContent, ...englishDocFiles()].map(normalizePath));
      },
      transform(code, id) {
        if (id.endsWith('.md')) {
          return `export default ${JSON.stringify(code)}`;
        }
      },
    },
  ],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
});
