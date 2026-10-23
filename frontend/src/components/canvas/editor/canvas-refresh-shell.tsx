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
          backgroundSize: "28px 28px",
        }}
      />

      <div
        className="absolute bottom-5 left-1/2 z-50 flex h-14 -translate-x-1/2 items-center gap-1 rounded-xl border px-2 shadow-lg backdrop-blur"
        style={{
          background: "var(--background)",
          borderColor: "var(--border)",
        }}
        aria-hidden="true"
      >
        {Array.from({ length: 7 }).map((_, index) => (
          <Skeleton key={index} className="size-8" />
        ))}
      </div>

      <div
        className="absolute bottom-24 left-6 z-50 h-40 w-canvas-menu rounded-lg border shadow-2xl backdrop-blur-sm"
        style={{
          background: "var(--background)",
          borderColor: "var(--border)",
        }}
        aria-hidden="true"
      >
        <Skeleton className="absolute top-7 left-7 h-5 w-12" />
        <Skeleton className="absolute top-16 left-28 h-6 w-16" />
        <Skeleton className="absolute bottom-7 left-16 h-8 w-20" />
        <div className="absolute inset-5 rounded border border-current opacity-15" />
      </div>

      <div
        className="w-image-dialog absolute bottom-5 left-5 z-50 flex h-14 items-center gap-2 rounded-xl border px-2 shadow-lg backdrop-blur"
        style={{
          background: "var(--background)",
          borderColor: "var(--border)",
        }}
        aria-hidden="true"
      >
        <Skeleton className="size-8" />
        <Skeleton className="size-8" />
        <Skeleton className="h-1 flex-1" />
        <Skeleton className="h-4 w-10" />
        <Skeleton className="size-8" />
      </div>
    </section>
  );
}
