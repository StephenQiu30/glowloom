import type { ComponentProps } from "react";
import { cn } from "cn";

export function Toolbar({
  className,
  density = "standard",
  ...props
}: ComponentProps<"div"> & {
  density?: "standard" | "roomy";
}) {
  return (
    <div
      data-slot="toolbar"
      data-toolbar-density={density}
      className={cn("flex min-w-0 items-center gap-2", className)}
      {...props}
    />
  );
}

export function ToolbarGroup({
  className,
  appearance = "framed",
  ...props
}: ComponentProps<"div"> & {
  appearance?: "framed" | "plain";
}) {
  return (
    <div
      role="group"
      data-slot="toolbar-group"
      data-appearance={appearance}
      className={cn("toolbar-group", className)}
      {...props}
    />
  );
}
