// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  transform: {
    '^.+\\.tsx?$': ['ts-jest', { isolatedModules: true, tsconfig: 'tsconfig.json' }],
  },
  testMatch: ['**/__tests__/**/*.test.ts'],
};
