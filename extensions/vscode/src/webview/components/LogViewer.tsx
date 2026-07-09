import { useT } from '../I18nProvider';
import { useRef, useEffect } from 'preact/hooks';
import { LogLine } from '../../shared/types';

interface Props { logs: LogLine[]; }

export function LogViewer({ logs }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const t = useT();

  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [logs.length]);

  return (
    <div class="log-viewer" ref={ref}>
      {logs.length === 0 ? (
        <div class="log-line"><span class="log-dim">{t('cmp.log.waiting')}</span><span class="log-cursor"></span></div>
      ) : (
        logs.map((l, i) => (
          <div class={`log-line ${l.level === 'warn' ? 'log-warn' : ''}`} key={i}>{l.text}</div>
        ))
      )}
    </div>
  );
}
