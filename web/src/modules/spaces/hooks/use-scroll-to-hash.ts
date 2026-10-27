import { useEffect, useRef } from 'react';
import { useLocation } from 'react-router';

const ring = 'ring-2 ring-paused'.split(' ');

export function useScrollToHash(ready: boolean) {
  const { hash } = useLocation();
  const done = useRef('');

  useEffect(() => {
    if (!ready || done.current === hash) return;
    const el = document.getElementById(hash.slice(1));
    if (!el) return;
    done.current = hash;
    el.scrollIntoView({ block: 'center' });
    el.classList.add(...ring);
    setTimeout(() => el.classList.remove(...ring), 2000);
  });
}
