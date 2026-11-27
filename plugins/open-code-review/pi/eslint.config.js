// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import neostandard from "neostandard"

export default [
  ...neostandard({
    ts: true,
    filesTs: ["src/**/*.ts"],
    ignores: ["dist/**", "node_modules/**"],
  }),
  {
    // Style here follows the other plugin in this repository, which writes double
    // quotes and trailing commas. The frontend and the docs site use a different
    // style, so the preset's Standard formatting is overridden locally instead of
    // assumed across the repository.
    rules: {
      "@stylistic/quotes": ["error", "double", { avoidEscape: true }],
      "@stylistic/comma-dangle": ["error", "always-multiline"],
      "@stylistic/space-before-function-paren": [
        "error",
        {
          anonymous: "never",
          named: "never",
          asyncArrow: "always",
          catch: "always",
        },
      ],
    },
  },
]
