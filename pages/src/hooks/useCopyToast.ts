// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useCallback, useEffect, useState } from 'react';

export function useCopyToast(duration = 1200) {
  const [toastVisible, setToastVisible] = useState(false);
  // Bumped on every successful copy so the hide timer restarts even when
  // the toast is already visible.
  const [copyCount, setCopyCount] = useState(0);

  const handleCopy = useCallback((text: string) => {
    const showToast = () => {
      setToastVisible(true);
      setCopyCount((n) => n + 1);
    };
    const fallbackCopy = (value: string) => {
      const textarea = document.createElement('textarea');
      textarea.value = value;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const success = document.execCommand('copy');
      document.body.removeChild(textarea);
      if (success) showToast();
    };

    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text)
        .then(showToast)
        .catch(() => fallbackCopy(text));
    } else {
      fallbackCopy(text);
    }
  }, []);

  useEffect(() => {
    if (!toastVisible) return;
    const timer = setTimeout(() => setToastVisible(false), duration);
    return () => clearTimeout(timer);
  }, [toastVisible, copyCount, duration]);

  return { toastVisible, handleCopy };
}
