// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

export const SIDEBAR_VIEW_ID = 'ocr.sidebar';
export const COMMENT_CONTROLLER_ID = 'ocr-review';

export const COMMANDS = {
  reviewStart: 'ocr.review.start',
  reviewCancel: 'ocr.review.cancel',
  configOpen: 'ocr.config.open',
  commentApply: 'ocr.comment.apply',
  commentDiscard: 'ocr.comment.discard',
  commentFalsePositive: 'ocr.comment.falsePositive',
} as const;
