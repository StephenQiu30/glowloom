import type { ComponentProps } from "react";
import { cn } from "cn";

export function ReferenceThumbnail({
  className,
  ...props
}: ComponentProps<"span">) {
  return (
    <span
      data-slot="reference-thumbnail"
      className={cn(
        "relative grid h-16 w-12 shrink-0 place-items-center overflow-hidden rounded-md bg-muted",
        className,
      )}
      {...props}
    />
  );
}
