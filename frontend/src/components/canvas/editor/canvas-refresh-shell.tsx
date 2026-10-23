import { Skeleton } from "@/components/ui/skeleton";

export function CanvasRefreshShell() {
  return (
    <section
      className="relative h-full min-h-0 overflow-hidden bg-background text-foreground"
      aria-busy="true"
      aria-label="正在加载画布"
    >
      <div
        className="absolute inset-0 opacity-60"
        style={{
          backgroundImage:
            "radial-gradient(circle, var(--border) 1px, transparent 1px)",
          backgroundSize: "20px 20px",
        }}
      />
      <div
        className="absolute top-3 left-3 flex h-11 items-center gap-3"
        aria-hidden="true"
      >
        <Skeleton className="size-6" />
        <Skeleton className="h-5 w-36" />
      </div>
      <div
        className="absolute top-1/2 left-3 flex max-h-[calc(100dvh-9rem)] w-12 -translate-y-1/2 flex-col items-center gap-3 overflow-hidden rounded-2xl border bg-popover py-3"
        aria-hidden="true"
      >
        {Array.from({ length: 8 }).map((_, index) => (
          <Skeleton key={index} className="size-8 shrink-0" />
        ))}
      </div>
      <div
        className="absolute bottom-3 left-4 flex h-11 items-center gap-2"
        aria-hidden="true"
      >
        <Skeleton className="size-9" />
        <Skeleton className="size-9" />
        <Skeleton className="h-5 w-16" />
      </div>
    </section>
  );
}
