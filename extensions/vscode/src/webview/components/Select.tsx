import { useT } from '../I18nProvider';
import { useState, useRef, useEffect } from 'preact/hooks';

export interface SelectOption {
  value: string;
  label: string;
}

interface Props {
  value: string;
  options: SelectOption[];
  placeholder?: string;
  onChange: (value: string) => void;
}

export function Select({ value, options, placeholder, onChange }: Props) {
  const t = useT();
  const resolvedPlaceholder = placeholder ?? t('cmp.select.placeholder');
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  const selected = options.find((o) => o.value === value);

  const pick = (v: string) => { onChange(v); setOpen(false); };

  return (
    <div class={`select${open ? ' open' : ''}`} ref={ref}>
      <button type="button" class="select-trigger" onClick={() => setOpen(!open)}>
        <span class={`select-value${selected ? '' : ' placeholder'}`}>
          {selected ? selected.label : resolvedPlaceholder}
        </span>
        <span class="select-arrow"></span>
      </button>
      {open && (
        <div class="select-menu">
          {options.map((o) => (
            <div
              key={o.value}
              class={`select-option${o.value === value ? ' active' : ''}`}
              onClick={() => pick(o.value)}
            >
              {o.label}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
