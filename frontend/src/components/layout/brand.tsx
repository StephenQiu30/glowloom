import Image from "next/image";
import Link from "next/link";
export function BrandMark({ monochrome = false }: { monochrome?: boolean }) {
  return (
    <span className="brand-mark" aria-hidden="true">
      {monochrome ? (
        <Image
          src="/design/figma/logo-mono.svg"
          alt=""
          width={32}
          height={32}
        />
      ) : (
        <>
          <Image
            className="brand-on-dark"
            src="/design/figma/logo-dark.svg"
            alt=""
            width={32}
            height={32}
          />
          <Image
            className="brand-on-light"
            src="/design/figma/logo-light.svg"
            alt=""
            width={32}
            height={32}
          />
        </>
      )}
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
