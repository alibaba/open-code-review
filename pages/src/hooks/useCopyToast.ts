import { useCallback, useEffect, useState } from 'react';

export function useCopyToast(duration = 1200) {
  const [toastVisible, setToastVisible] = useState(false);

  const handleCopy = useCallback((text: string) => {
    const fallbackCopy = (value: string) => {
      const textarea = document.createElement('textarea');
      textarea.value = value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const success = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (success) setToastVisible(true);
    };

    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text)
        .then(() => setToastVisible(true))
        .catch(() => fallbackCopy(text));
    } else {
      fallbackCopy(text);
    }
  }, []);

  useEffect(() => {
    if (!toastVisible) return;
    const timer = setTimeout(() => setToastVisible(false), duration);
    return () => clearTimeout(timer);
  }, [toastVisible, duration]);

  return { toastVisible, handleCopy };
}
