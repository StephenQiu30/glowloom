import type { CSSProperties } from "react";
import { cn } from "cn";

const sizes = {
  creation: 20,
  explore: 20,
  assets: 20,
  subjects: 20,
  canvas: 14,
  sidebar: 16,
  notification: 16,
  store: 16,
  credit: 12,
  "new-creation": 16,
  plus: 16,
} as const;

/** Shared Figma vectors, kept at their exported dimensions and tinted by context. */
export function ProductNavigationIcon({
  name,
  className,
}: {
  name: keyof typeof sizes;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      data-product-icon={name}
      className={cn("product-navigation-icon", className)}
      style={
        {
          "--navigation-icon-size": `${sizes[name]}px`,
          maskImage: `url(/icons/navigation/${name}.svg)`,
          WebkitMaskImage: `url(/icons/navigation/${name}.svg)`,
          ...(name === "new-creation"
            ? { maskSize: "75%", WebkitMaskSize: "75%" }
            : name === "plus"
              ? { maskSize: "76.67%", WebkitMaskSize: "76.67%" }
              : {}),
        } as CSSProperties
      }
    />
  );
}
