import { useId } from "react";
/** Original vector scene, intentionally distinct from the reference product's film photographs. */
export function CinemaPreview({
  variant = 0,
  colors,
}: {
  variant?: number;
  colors?: string[];
}) {
  const id = useId().replace(/:/g, "");
  const paint = (i: number, fallback: string) =>
    colors?.[i % colors.length] ?? fallback;
  return (
    <svg
      viewBox="0 0 320 180"
      role="img"
      aria-label="独立场景示意预览"
      className="cinema-preview"
    >
      <defs>
        <linearGradient id={id} x2="0" y2="1">
          <stop stopColor={paint(0, "var(--cinema-preview-sky)")} />
          <stop offset="1" stopColor={paint(1, "var(--cinema-preview-road)")} />
        </linearGradient>
      </defs>
      <rect width="320" height="180" fill={`url(#${id})`} />
      <path
        d="M0 180 120 100H200L320 180Z"
        fill={paint(3, "var(--cinema-preview-road)")}
      />
      <path
        d="M0 0H100V120L0 160Z"
        fill={paint(2, "var(--cinema-preview-wall)")}
      />
      <path
        d="M220 0H320V160L220 120Z"
        fill={paint(4, "var(--cinema-preview-wall)")}
      />
      {[0, 1, 2, 3].map((i) => (
        <path
          key={i}
          d={`M${12 + i * 21} 24v70M${231 + i * 21} 24v70`}
          stroke={paint(5, "var(--cinema-preview-window)")}
          strokeWidth="12"
        />
      ))}
      <circle
        cx={155 + (variant % 3) * 7}
        cy="76"
        r="12"
        fill={paint(6, "var(--cinema-preview-person)")}
      />
      <path
        d={`M${155 + (variant % 3) * 7} 88v43m0-28-21 18m21-18 22 14m-22 14-17 34m17-34 23 31`}
        stroke={paint(6, "var(--cinema-preview-person)")}
        strokeWidth="11"
        strokeLinecap="round"
      />
      <path
        d="M131 162h45"
        stroke={paint(7, "var(--cinema-preview-line)")}
        strokeWidth="2"
      />
      {variant % 2 === 1 ? (
        <path
          d="M18 150h75m-12-9 12 9-12 9"
          fill="none"
          stroke="var(--cinema-preview-line)"
          strokeWidth="3"
        />
      ) : null}
    </svg>
  );
}
