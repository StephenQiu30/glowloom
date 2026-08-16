import { type HTMLAttributes } from "react";

import { cn } from "@/lib/class-names";

export const layoutContainerClassName =
  "mx-auto w-full max-w-[1440px] px-5 md:px-8";

export function LayoutContainer({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn(layoutContainerClassName, className)} {...props} />;
}
