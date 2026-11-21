// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { ConfigPathStatus } from '@shared/types';

export class CliUpgradeRequiredError extends Error {
  constructor() {
    super('This OCR CLI does not support config path --json. Upgrade it with: npm install -g @alibaba-group/open-code-review');
  }
}

export function configPathFailure(error: unknown): ConfigPathStatus {
  return {
    ok: false,
    message: error instanceof Error ? error.message : String(error),
    upgradeRequired: error instanceof CliUpgradeRequiredError,
  };
}
