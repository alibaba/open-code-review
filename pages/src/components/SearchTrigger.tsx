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
        background: 'rgba(255,255,255,0.04)',
        border: '1px solid rgba(255,255,255,0.12)',
        borderRadius: 8,
        cursor: 'pointer',
        color: 'rgba(255,255,255,0.4)',
        fontSize: 14,
        fontFamily,
        outline: 'none',
        transition: 'border-color 0.15s',
        ...style,
      }}
      onMouseEnter={e => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.25)')}
      onMouseLeave={e => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.12)')}
    >
      <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <img src={searchIcon} alt="" style={{ width: 16, height: 16, opacity: 0.6 }} />
        {placeholder}
      </span>
      <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.3)', fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', lineHeight: 1 }}>
        {navigator.userAgent?.includes('Mac') ? '⌘K' : 'Ctrl+K'}
      </span>
    </button>
  );
}
