import { useState } from 'react';

export function useStoredFlag(key: string) {
  const [on, setOn] = useState(() => {
    try {
      return localStorage.getItem(key) === '1';
    } catch {
      return false;
    }
  });
  const toggle = () => {
    const next = !on;
    setOn(next);
    try {
      localStorage.setItem(key, next ? '1' : '0');
    } catch {
      // storage can be blocked; the choice then lasts until reload
    }
  };
  return [on, toggle] as const;
}
