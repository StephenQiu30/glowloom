"use client";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { libraryKey } from "./library-queries";
import { getLibraryStorageUsage } from "./library-storage-query";
import type { LibraryIdentity } from "./library-model";
export function formatStorageBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), 4);
  return `${(bytes / 1024 ** unit).toLocaleString("zh-CN", { maximumFractionDigits: 2 })} ${["B", "KiB", "MiB", "GiB", "TiB"][unit]}`;
}
export function LibraryStorageMeter({
  identity,
}: {
  identity: LibraryIdentity;
}) {
  const query = useQuery({
    queryKey: [...libraryKey(identity), "storage-usage"],
    queryFn: ({ signal }) => getLibraryStorageUsage(identity, signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
  });
  const usage = query.isError || query.isFetching ? undefined : query.data;
  return (
    <section
      aria-label="素材库存储容量"
      className="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-surface-2 p-4"
    >
      <div className="min-w-0 text-sm">
        <p className="font-medium">
          {identity.scope.kind === "personal" ? "个人素材存储" : "项目素材存储"}
        </p>
        {query.isFetching ? (
          <p role="status">正在核对实际对象容量…</p>
        ) : query.isError ? (
          <p role="alert">容量暂时不可读取：{query.error.message}</p>
        ) : usage ? (
          <>
            <p>
              已占用 {formatStorageBytes(usage.used_bytes)} ·{" "}
              {usage.object_count.toLocaleString("zh-CN")} 个独立对象
              {usage.limit_bytes === null
                ? " · 未配置容量上限"
                : ` / 上限 ${formatStorageBytes(usage.limit_bytes)}`}
            </p>
            <p className="text-muted-foreground">
              包含尚未实际清理的回收素材与待核实对象 · 核对时间{" "}
              {new Date(usage.calculated_at).toLocaleString("zh-CN")}
            </p>
          </>
        ) : null}
      </div>
      <Button
        size="sm"
        variant="outline"
        disabled={query.isFetching}
        onClick={() => void query.refetch()}
      >
        刷新实际容量
      </Button>
    </section>
  );
}
