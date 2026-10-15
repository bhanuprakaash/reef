import { useNavigate } from 'react-router';

export function BackButton({ fallback }: { fallback: string }) {
  const navigate = useNavigate();
  return (
    <button
      type="button"
      aria-label="Back"
      title="Back"
      onClick={() => (window.history.state?.idx > 0 ? navigate(-1) : navigate(fallback))}
      className="flex size-8 shrink-0 max-md:size-11 items-center justify-center rounded-lg text-soft hover:bg-hover/50"
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <path d="M15 5l-7 7 7 7" />
      </svg>
    </button>
  );
}
