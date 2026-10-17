import { useSyncExternalStore } from 'react';

const query = '(prefers-color-scheme: light)';

function subscribe(notify: () => void) {
  const media = window.matchMedia(query);
  const observer = new MutationObserver(notify);
  observer.observe(document.documentElement, { attributeFilter: ['data-theme'] });
  media.addEventListener('change', notify);
  return () => {
    observer.disconnect();
    media.removeEventListener('change', notify);
  };
}

function snapshot(): 'dark' | 'light' {
  const theme = document.documentElement.dataset.theme;
  return theme === 'light' || (theme === 'system' && window.matchMedia(query).matches)
    ? 'light'
    : 'dark';
}

// useResolvedTheme is the theme the window shows now, for drawing that CSS cannot reach
// (canvas, generated SVG).
export function useResolvedTheme() {
  return useSyncExternalStore(subscribe, snapshot, () => 'dark' as const);
}
