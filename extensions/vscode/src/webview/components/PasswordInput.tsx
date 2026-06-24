import { useState } from 'preact/hooks';

interface Props {
  value: string;
  placeholder?: string;
  className?: string;
  onInput: (value: string) => void;
}

function EyeIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function EyeOffIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
      <path d="M1 1l22 22" />
      <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
    </svg>
  );
}

export function PasswordInput({ value, placeholder, className, onInput }: Props) {
  const [visible, setVisible] = useState(false);

  return (
    <div class="password-input-wrap">
      <input
        class={`form-input password-input${className ? ` ${className}` : ''}`}
        type={visible ? 'text' : 'password'}
        value={value}
        placeholder={placeholder}
        onInput={(e) => onInput((e.target as HTMLInputElement).value)}
      />
      <button
        type="button"
        class="password-toggle"
        onClick={() => setVisible(!visible)}
        aria-label={visible ? '隐藏密钥' : '显示密钥'}
        tabIndex={-1}
      >
        {visible ? <EyeOffIcon /> : <EyeIcon />}
      </button>
    </div>
  );
}
