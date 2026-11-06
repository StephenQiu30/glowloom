import type { ComponentProps } from "react";
import { Toolbar } from "@/components/ui/toolbar";
import { cn } from "@/lib/utils";

export function FilterToolbar({ className, ...props }: ComponentProps<"div">) {
  return (
    <Toolbar className={cn("workspace-filter-toolbar", className)} {...props} />
  );
}
