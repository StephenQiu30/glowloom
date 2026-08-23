import Image from "next/image";
import Link from "next/link";

import { cn } from "@/lib/class-names";

const studioBrandSizes = {
  l: "h-7",
  lg: "h-9",
  xl: "h-9.5",
} as const;

const studioBrandMediumSizes = {
  l: "md:h-7",
  lg: "md:h-9",
  xl: "md:h-9.5",
} as const;

type StudioBrandSize = keyof typeof studioBrandSizes;

export function StudioBrand({
  size = "lg",
  mdSize,
}: {
  size?: StudioBrandSize;
  mdSize?: StudioBrandSize;
}) {
  return (
    <Link
      className="inline-flex shrink-0 items-center"
      href="/"
      aria-label="Lanverse 首页"
    >
      <Image
        alt="Lanverse"
        className={cn(
          "w-auto dark:invert",
          studioBrandSizes[size],
          mdSize ? studioBrandMediumSizes[mdSize] : null,
        )}
        height={402}
        priority
        src="/brand/lanverse-logo.png"
        width={1667}
      />
    </Link>
  );
}
