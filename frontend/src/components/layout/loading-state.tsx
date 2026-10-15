import type { ReactNode } from "react";
import { cn } from "cn";
import { Spinner } from "@/components/ui/spinner";

/** Announce the real pending operation once; the spinner is decorative. */
export function LoadingState({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      role="status"
      className={cn(
        "flex min-w-0 items-center gap-2 text-sm text-muted-foreground",
        className,
      )}
    >
      <Spinner aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
}
