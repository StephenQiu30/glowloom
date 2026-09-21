import { Skeleton } from "@/components/ui/skeleton";

export function PageLoading({ label = "正在加载页面" }: { label?: string }) {
  return (
    <div role="status" aria-label={label} className="flex min-h-64 w-full flex-col gap-6 py-8">
      <span className="sr-only">{label}</span>
      <Skeleton className="h-8 w-40" />
      <Skeleton className="h-4 w-2/3" />
      <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3" aria-hidden="true">
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    </div>
  );
}
