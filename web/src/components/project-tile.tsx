export function ProjectTile({
  letter,
  colour,
  size = 24,
}: {
  letter?: string;
  colour?: string;
  size?: number;
}) {
  return (
    <span
      aria-hidden="true"
      style={{ width: size, height: size, fontSize: size * 0.52, background: colour }}
      className={`flex flex-none items-center justify-center rounded-[7px] font-semibold ${colour ? 'text-white' : 'bg-bubble text-soft'}`}
    >
      {letter ?? (
        <svg
          width={size * 0.58}
          height={size * 0.58}
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinejoin="round"
        >
          <rect x="4" y="4" width="6.5" height="6.5" rx="1.6" />
          <rect x="13.5" y="4" width="6.5" height="6.5" rx="1.6" />
          <rect x="4" y="13.5" width="6.5" height="6.5" rx="1.6" />
          <rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.6" />
        </svg>
      )}
    </span>
  );
}
