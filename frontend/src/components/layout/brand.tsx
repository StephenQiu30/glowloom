import Link from "next/link";
export function BrandMark({ monochrome = false }: { monochrome?: boolean }) {
  return (
    <span className="brand-mark" aria-hidden="true">
      <svg
        viewBox="0 0 32 32"
        fill="none"
        focusable="false"
        data-monochrome={monochrome}
      >
        <path
          className="brand-warp"
          d="M16 2L30 16L25 21L21 17L22 16L16 10L13 13L9 9L16 2Z"
        />
        <path
          fill="currentColor"
          d="M16 30L2 16L7 11L11 15L10 16L16 22L19 19L23 23L16 30Z"
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
