// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { resolveLocale, t, toHtmlLang } from '../i18n';

describe('Turkish display locale', () => {
  test.each(['tr', 'tr-tr', 'tr-TR', 'TR'])('resolves %s and renders Turkish UI', raw => {
    const locale = resolveLocale(raw);
    expect(locale).toBe('tr-tr');
    expect(toHtmlLang(locale)).toBe('tr-TR');
    expect(t(locale, 'ext.comment.applyWorkspaceOnly')).not.toBe(t('en', 'ext.comment.applyWorkspaceOnly'));
  });

  test('preserves interpolation parameters and unknown-key fallback', () => {
    expect(t('tr-tr', 'ext.deleteProviderConfirm')).toContain('{name}');
    expect(t('tr-tr', 'ext.comment.jumpLineUnresolved')).toContain('{path}');
    expect(t('tr-tr', 'unknown.key')).toBe('unknown.key');
    expect(resolveLocale('unsupported')).toBe('en');
  });
});
