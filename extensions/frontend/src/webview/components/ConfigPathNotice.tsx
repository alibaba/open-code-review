// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { ConfigPathStatus } from '../../shared/types';
import { useT } from '../I18nProvider';

export function ConfigPathNotice({ status, onOpen }: { status: ConfigPathStatus | null; onOpen: () => void }) {
  const t = useT();
  if (!status || status.ok) return null;
  return <div role="alert" class="wizard-body">
    <p>{status.message || t('view.env.pathFailed')}</p>
    <button type="button" class="btn-primary" onClick={onOpen}>
      {t(status.upgradeRequired ? 'view.env.upgradeCli' : 'view.env.resolveConfig')}
    </button>
  </div>;
}
