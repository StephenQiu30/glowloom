"use client";

import type { ComponentProps } from "react";
import { TabsList } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

/** Page navigation uses the official Tabs keyboard and focus behavior. */
export function PageTabsList({
  className,
  ...props
}: ComponentProps<typeof TabsList>) {
  return (
    <TabsList className={cn("workspace-page-tabs", className)} {...props} />
  );
}
