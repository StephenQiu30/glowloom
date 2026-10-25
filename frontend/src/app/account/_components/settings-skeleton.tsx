import { Skeleton } from "@/components/ui/skeleton";
export function SettingsSkeleton({
  label = "正在读取账号设置",
}: {
  label?: string;
}) {
  return (
    <div role="status" aria-label={label} className="flex flex-col gap-5">
      <span className="sr-only">{label}</span>
      <div className="flex h-14 items-center gap-3">
        <Skeleton className="size-12 rounded-full" />
        <div className="flex flex-1 flex-col gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3 w-36" />
        </div>
        <Skeleton className="h-9 w-20" />
      </div>
      <div className="flex h-24 flex-col justify-around">
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-full" />
      </div>
      <div className="flex justify-between">
        <Skeleton className="h-9 w-20" />
        <Skeleton className="h-9 w-20" />
      </div>
    </div>
  );
}
