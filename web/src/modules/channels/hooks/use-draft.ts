import { useState } from 'react';

function read(key: string) {
  try {
    return localStorage.getItem(key) ?? '';
  } catch {
    return '';
  }
}

export function useDraft(key: string) {
  const [state, setState] = useState({ key, text: read(key) });
  let text = state.text;
  if (state.key !== key) {
    text = read(key);
    setState({ key, text });
  }
  const setText = (next: string) => {
    setState({ key, text: next });
    try {
      if (next === '') localStorage.removeItem(key);
      else localStorage.setItem(key, next);
    } catch {
      // storage can be blocked; the draft then lasts until reload
    }
  };
  return [text, setText] as const;
}
