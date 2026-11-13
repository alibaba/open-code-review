// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import searchIcon from '../assets/icons/icon-search.svg';

const fontFamily = 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif';

interface SearchTriggerProps {
  placeholder: string;
  onClick: () => void;
  style?: React.CSSProperties;
}

export function SearchTrigger({ placeholder, onClick, style }: SearchTriggerProps) {
  return (
    <button
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 12px',
        background: 'var(--fill-subtle)',
        border: '1px solid var(--line)',
        borderRadius: 8,
        cursor: 'pointer',
        color: 'var(--text-ghost)',
        fontSize: 14,
        fontFamily,
        outline: 'none',
        transition: 'border-color 0.15s',
        ...style,
      }}
      onMouseEnter={e => (e.currentTarget.style.borderColor = 'var(--line-strong)')}
      onMouseLeave={e => (e.currentTarget.style.borderColor = 'var(--line)')}
    >
      <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <img src={searchIcon} alt="" style={{ width: 16, height: 16, opacity: 0.6 }} />
        {placeholder}
      </span>
      <span style={{ fontSize: 13, color: 'var(--text-faintest)', fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', lineHeight: 1 }}>
        {navigator.userAgent?.includes('Mac') ? '⌘K' : 'Ctrl+K'}
      </span>
    </button>
  );
}
