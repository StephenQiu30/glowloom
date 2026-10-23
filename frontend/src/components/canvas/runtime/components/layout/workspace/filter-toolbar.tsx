import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export function FilterToolbar({ className, ...props }: ComponentProps<"div">) {
  return (
    <div className={cn("workspace-filter-toolbar", className)} {...props} />
  );
}
