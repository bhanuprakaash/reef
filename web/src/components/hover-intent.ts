export type HoverIntent = ReturnType<typeof createHoverIntent>;

// enter opens after delay; leave closes after grace so the pointer can cross the gap onto
// the card; focus, blur and dismiss act at once. Any call cancels the one pending before it.
export function createHoverIntent(onChange: (open: boolean) => void, delay: number, grace = 100) {
  let open = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function set(next: boolean, wait: number) {
    clearTimeout(timer);
    if (next === open) return;
    const apply = () => {
      open = next;
      onChange(next);
    };
    if (wait === 0) apply();
    else timer = setTimeout(apply, wait);
  }

  return {
    enter: () => set(true, delay),
    focus: () => set(true, 0),
    leave: () => set(false, grace),
    blur: () => set(false, 0),
    dismiss: () => set(false, 0),
    cancel: () => clearTimeout(timer),
  };
}
