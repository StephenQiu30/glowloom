import type { ReactNode } from "react";
import { cn } from "cn";
import { Spinner } from "@/components/ui/spinner";
import { Skeleton } from "@/components/ui/skeleton";

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
      <Spinner role="presentation" aria-hidden="true" />
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
}

/** Neutral content placeholder; navigation stays available outside this boundary. */
export function PageLoading({ message = "正在加载…" }: { message?: string }) {
  return (
    <section
      aria-busy="true"
      className="flex flex-col gap-6 in-data-[full-bleed=true]:p-4 md:in-data-[full-bleed=true]:px-10 md:in-data-[full-bleed=true]:py-6"
    >
      <LoadingState>{message}</LoadingState>
      <div aria-hidden="true" className="flex flex-col gap-4">
        <Skeleton className="h-8 w-1/4" />
        <Skeleton className="h-64 w-full" />
      </div>
    </section>
  );
}
