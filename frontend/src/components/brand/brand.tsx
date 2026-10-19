import Link from "next/link";
export function BrandMark({ monochrome = true }: { monochrome?: boolean }) {
  return (
    <span className="brand-mark" aria-hidden="true">
      <svg
        viewBox="0 0 32 32"
        fill="none"
        focusable="false"
        data-monochrome={monochrome}
      >
        <path
          fill="currentColor"
          d="M16 2L30 16L24 22L18 16L16 18L22 24L16 30L2 16L8 10L14 16L16 14L10 8Z"
        />
        <path
          className="brand-warp"
          d="M16 2L30 16L24 22L18 16L16 18L14 16L16 14L10 8Z"
        />
      </svg>
    </span>
  );
}
export function Brand({ wordmark = false }: { wordmark?: boolean }) {
  return (
    <Link href="/" aria-label="织光首页" className="product-brand">
      <BrandMark />
      {wordmark && (
        <>
          <span>织光</span>
          <small>Glowloom</small>
        </>
      )}
    </Link>
  );
}
