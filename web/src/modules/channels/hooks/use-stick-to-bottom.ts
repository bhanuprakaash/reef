import { useEffect, useRef } from 'react';

const _nearBottom = 120;

// Keeps the box scrolled to the end as its content grows, but only while the user is near the end,
// or when `jump` changes (they just sent a message).
export function useStickToBottom(jump?: unknown) {
  const box = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);

  const onScroll = () => {
    const el = box.current;
    if (el) stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight < _nearBottom;
  };

  useEffect(() => {
    const el = box.current;
    if (!el?.firstElementChild) return;
    const observer = new ResizeObserver(() => {
      if (stuck.current) el.scrollTop = el.scrollHeight;
    });
    observer.observe(el.firstElementChild);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    stuck.current = true;
    el.scrollTop = el.scrollHeight;
  }, [jump]);

  return { box, onScroll };
}
