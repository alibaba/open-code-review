module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  transform: {
    '^.+\\.tsx?$': ['ts-jest', { isolatedModules: true, tsconfig: 'tsconfig.extension.json' }],
  },
  testMatch: ['**/__tests__/**/*.test.ts'],
};
