// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useT } from '../I18nProvider';
export function CancelledView() {
  const t = useT();
  return (
    <div class="action-cancelled" style="display:block">
      <div class="cancelled-note">{t('view.cancelled.title')}</div>
    </div>
  );
}
