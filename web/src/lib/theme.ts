export type Theme = 'dark' | 'light' | 'system';

export const THEME_KEY = 'reef.theme';

export function getTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_KEY);
    return stored === 'light' || stored === 'system' ? stored : 'dark';
  } catch {
    return 'dark';
  }
}

export function setTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  try {
    localStorage.setItem(THEME_KEY, theme);
  } catch {
    // storage can be blocked; the choice then lasts until reload
  }
}
