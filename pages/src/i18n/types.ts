// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { en } from './en';

export type Language = 'en' | 'zh' | 'ja' | 'ru';
export type TranslationKey = keyof typeof en;
export type TranslationKeys = Record<TranslationKey, string>;
