// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import githubIcon from '../assets/icons/icon-github.svg';
import { LanguageProvider } from '../i18n';
import BenchmarkSection from './BenchmarkSection';

it('links the leaderboard to the AACR-Bench repository', () => {
  render(<LanguageProvider><BenchmarkSection /></LanguageProvider>);

  const link = screen.getByRole('link', { name: 'AACR-Bench' });
  expect(link).toHaveAttribute('href', 'https://github.com/alibaba/aacr-bench');
  expect(link).toHaveAttribute('target', '_blank');
  expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  expect(within(link).getByRole('presentation')).toHaveAttribute('src', githubIcon);
});
