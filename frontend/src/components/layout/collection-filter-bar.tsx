import type { ComponentProps } from "react";
import { cn } from "cn";
import { Toolbar } from "@/components/ui/toolbar";
import "./collection-filter-bar.css";

export function CollectionFilterBar({
  className,
  ...props
}: ComponentProps<typeof Toolbar>) {
  return (
    <Toolbar
      {...props}
      role="group"
      className={cn("collection-filter-bar", className)}
    />
  );
}
